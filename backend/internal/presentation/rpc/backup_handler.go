package rpc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// BackupHandler は BackupService のハンドラ。
type BackupHandler struct {
	mcadminv1connect.UnimplementedBackupServiceHandler

	backups *backupctl.UseCase
}

// NewBackupHandler は BackupHandler を作る。
func NewBackupHandler(b *backupctl.UseCase) *BackupHandler { return &BackupHandler{backups: b} }

// ListBackups は保管済みのバックアップを新しい順で返す。
func (h *BackupHandler) ListBackups(
	ctx context.Context,
	_ *connect.Request[mcadminv1.ListBackupsRequest],
) (*connect.Response[mcadminv1.ListBackupsResponse], error) {
	listing, err := h.backups.List(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}

	out := &mcadminv1.ListBackupsResponse{
		Backups:        make([]*mcadminv1.Backup, len(listing.Backups)),
		Directory:      listing.Directory,
		TotalSizeBytes: listing.TotalSizeBytes,
	}
	for i, b := range listing.Backups {
		out.Backups[i] = backupToProto(b)
	}
	return connect.NewResponse(out), nil
}

// CreateBackup はバックアップを取得する。
func (h *BackupHandler) CreateBackup(
	ctx context.Context,
	req *connect.Request[mcadminv1.CreateBackupRequest],
) (*connect.Response[mcadminv1.CreateBackupResponse], error) {
	handle, err := h.backups.Create(ctx, backupModeFromProto(req.Msg.GetMode()), req.Msg.GetNote())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.CreateBackupResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// DeleteBackup はバックアップを 1 件削除する。
func (h *BackupHandler) DeleteBackup(
	ctx context.Context,
	req *connect.Request[mcadminv1.DeleteBackupRequest],
) (*connect.Response[mcadminv1.DeleteBackupResponse], error) {
	freed, err := h.backups.Delete(ctx, req.Msg.GetBackupId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.DeleteBackupResponse{FreedBytes: freed}), nil
}

// GetRetentionPolicy は現在の保持ポリシーを返す。
func (h *BackupHandler) GetRetentionPolicy(
	ctx context.Context,
	_ *connect.Request[mcadminv1.GetRetentionPolicyRequest],
) (*connect.Response[mcadminv1.GetRetentionPolicyResponse], error) {
	policy, err := h.backups.GetRetentionPolicy(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.GetRetentionPolicyResponse{
		Policy: retentionPolicyToProto(policy),
	}), nil
}

// SetRetentionPolicy は保持ポリシーを .env へ書き戻す。
func (h *BackupHandler) SetRetentionPolicy(
	ctx context.Context,
	req *connect.Request[mcadminv1.SetRetentionPolicyRequest],
) (*connect.Response[mcadminv1.SetRetentionPolicyResponse], error) {
	requested := req.Msg.GetPolicy()
	policy, err := h.backups.SetRetentionPolicy(ctx,
		int(requested.GetKeepCount()), int(requested.GetKeepDays()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.SetRetentionPolicyResponse{
		Policy: retentionPolicyToProto(policy),
	}), nil
}

// PruneBackups は保持ポリシーを手動で適用する。
func (h *BackupHandler) PruneBackups(
	ctx context.Context,
	req *connect.Request[mcadminv1.PruneBackupsRequest],
) (*connect.Response[mcadminv1.PruneBackupsResponse], error) {
	result, err := h.backups.Prune(ctx, req.Msg.GetDryRun())
	if err != nil {
		return nil, toConnectError(err)
	}

	ids := make([]string, len(result.DeletedIDs))
	for i, id := range result.DeletedIDs {
		ids[i] = id.String()
	}
	return connect.NewResponse(&mcadminv1.PruneBackupsResponse{
		DeletedIds: ids,
		FreedBytes: result.FreedBytes,
	}), nil
}

// PreflightRestore は復元前の確認を返す。副作用を持たない。
func (h *BackupHandler) PreflightRestore(
	ctx context.Context,
	req *connect.Request[mcadminv1.PreflightRestoreRequest],
) (*connect.Response[mcadminv1.PreflightRestoreResponse], error) {
	pre, err := h.backups.PreflightRestore(ctx, req.Msg.GetBackupId())
	if err != nil {
		return nil, toConnectError(err)
	}

	return connect.NewResponse(&mcadminv1.PreflightRestoreResponse{
		Backup:               backupToProto(pre.Backup),
		CurrentLevel:         pre.CurrentLevel.String(),
		CurrentWorldVersion:  worldVersionToProto(pre.CurrentVersion),
		ConfiguredMcVersion:  pre.ConfiguredVersion,
		Verdict:              verdictToProto(pre.Decision.Verdict),
		LevelNameMismatch:    pre.Decision.LevelNameMismatch,
		Warnings:             pre.Decision.Warnings,
		RequiresConfirmation: pre.Decision.RequiresConfirmation,
		RequiredBytes:        pre.RequiredBytes,
		AvailableBytes:       pre.AvailableBytes,
	}), nil
}

// RestoreBackup は復元を実行する。
//
// 画面が事前確認を通ったと主張しても信用せず、ユースケースが
// サーバー側で PreflightRestore を再実行してから進む（REQ-113）。
func (h *BackupHandler) RestoreBackup(
	ctx context.Context,
	req *connect.Request[mcadminv1.RestoreBackupRequest],
) (*connect.Response[mcadminv1.RestoreBackupResponse], error) {
	handle, err := h.backups.Restore(ctx, backupctl.RestoreRequest{
		BackupID:                  req.Msg.GetBackupId(),
		Target:                    restoreTargetFromProto(req.Msg.GetTarget()),
		AcknowledgeVersionWarning: req.Msg.GetAcknowledgeVersionWarning(),
		ConfirmLevelName:          req.Msg.GetConfirmLevelName(),
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.RestoreBackupResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// restoreTargetFromProto は復元先の指定を写す。未指定はアーカイブのワールド名。
func restoreTargetFromProto(t mcadminv1.RestoreTarget) backupctl.RestoreTarget {
	if t == mcadminv1.RestoreTarget_RESTORE_TARGET_CURRENT_LEVEL {
		return backupctl.TargetCurrentLevel
	}
	return backupctl.TargetArchiveLevel
}

func verdictToProto(v backup.Verdict) mcadminv1.VersionVerdict {
	switch v {
	case backup.VerdictMatch:
		return mcadminv1.VersionVerdict_VERSION_VERDICT_MATCH
	case backup.VerdictOlderWillUpgrade:
		return mcadminv1.VersionVerdict_VERSION_VERDICT_OLDER_WILL_UPGRADE
	case backup.VerdictNewerIncompatible:
		return mcadminv1.VersionVerdict_VERSION_VERDICT_NEWER_INCOMPATIBLE
	case backup.VerdictUnknown:
		return mcadminv1.VersionVerdict_VERSION_VERDICT_UNKNOWN
	default:
		return mcadminv1.VersionVerdict_VERSION_VERDICT_UNSPECIFIED
	}
}

// backupModeFromProto は取得方式を写す。未指定は HOT。
// 既定を HOT にするのは docs/backup-restore.md の検証済み手順に合わせるため。
func backupModeFromProto(m mcadminv1.BackupMode) backupctl.Mode {
	if m == mcadminv1.BackupMode_BACKUP_MODE_COLD {
		return backupctl.ModeCold
	}
	return backupctl.ModeHot
}

func backupToProto(b backupctl.Entry) *mcadminv1.Backup {
	out := &mcadminv1.Backup{
		Id:              b.ID.String(),
		SizeBytes:       b.SizeBytes,
		DeclaredVersion: b.DeclaredVersion,
		ArchiveLevel:    b.ArchiveLevel,
		Version:         worldVersionToProto(b.Version),
		EntryRoots:      b.EntryRoots,
	}
	if !b.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(b.CreatedAt)
	}
	return out
}

func retentionPolicyToProto(p backup.RetentionPolicy) *mcadminv1.RetentionPolicy {
	return &mcadminv1.RetentionPolicy{
		KeepCount: int32(p.KeepCount()),
		KeepDays:  int32(p.KeepDays()),
	}
}
