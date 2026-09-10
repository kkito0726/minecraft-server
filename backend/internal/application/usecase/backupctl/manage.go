package backupctl

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// Entry は一覧に出すバックアップ 1 件。
//
// DeclaredVersion と DeclaredLevel はファイル名から読み取った参考値である。
// 人がファイルを改名できるため、これを真の値として扱ってはならない。
// 本当のバージョンとワールド名はアーカイブ内の level.dat から読む（REQ-413）。
type Entry struct {
	ID        backup.ID
	SizeBytes int64
	// CreatedAt はファイルの更新時刻。ファイル名の日時ではない。
	CreatedAt       time.Time
	DeclaredVersion string
	DeclaredLevel   string
	// ArchiveLevel はエントリの接頭辞から判定した実際のワールド名。
	ArchiveLevel string
	// EntryRoots は含まれるルート（data/world、data/plugins など）。
	EntryRoots []string
	// Version はアーカイブ内の level.dat から読んだ値。これが真のバージョン。
	// 読めなければ「読めない」を表す値になる（EDGE-001）。
	Version shared.WorldVersion
}

// ListResult は一覧の結果。
type ListResult struct {
	// Backups は新しい順。
	Backups        []Entry
	Directory      string
	TotalSizeBytes int64
}

// List は保管済みのバックアップを新しい順で返す。
//
// ファイル名を解釈できないものも一覧に出す。人が付けた名前や、
// 旧形式で取ったものを画面から消してしまうと、削除する手段も失われる。
func (u *UseCase) List(ctx context.Context) (ListResult, error) {
	stored, err := u.cfg.Store.List(ctx)
	if err != nil {
		return ListResult{}, err
	}

	entries := make([]Entry, 0, len(stored))
	var total int64
	for _, s := range stored {
		info := backup.ParseName(s.ID.String())
		archived := u.inspect(ctx, s.ID)
		entries = append(entries, Entry{
			ID:              s.ID,
			SizeBytes:       s.SizeBytes,
			CreatedAt:       s.CreatedAt,
			DeclaredVersion: info.Version,
			DeclaredLevel:   info.Level,
			ArchiveLevel:    archived.level,
			EntryRoots:      archived.roots,
			Version:         archived.version,
		})
		total += s.SizeBytes
	}

	// 保管先の返す順序には依存しない。
	slices.SortFunc(entries, func(a, b Entry) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})

	return ListResult{
		Backups:        entries,
		Directory:      u.cfg.Store.Directory(),
		TotalSizeBytes: total,
	}, nil
}

// archived はアーカイブの中身から読み取った値。
type archived struct {
	level   string
	roots   []string
	version shared.WorldVersion
}

// inspect はアーカイブの中身を読む。読めなければ空の値を返す。
//
// 壊れたアーカイブが 1 つあっても一覧は成立させる（EDGE-001）。
// 読めないことは「読めない」として画面に出し、削除する手段は残す。
func (u *UseCase) inspect(ctx context.Context, id backup.ID) archived {
	unreadable := archived{version: shared.UnreadableWorldVersion()}

	info, err := u.cfg.Store.Inspect(ctx, id)
	if err != nil {
		return unreadable
	}
	result := archived{level: info.Level, roots: info.EntryRoots, version: shared.UnreadableWorldVersion()}
	if !info.HasLevelDat || u.cfg.Levels == nil {
		return result
	}

	rc, err := u.cfg.Store.OpenLevelDat(ctx, id)
	if err != nil {
		return result
	}
	defer func() { _ = rc.Close() }()

	result.version = u.cfg.Levels.Read(ctx, rc)
	return result
}

// Delete はバックアップを 1 件削除し、解放されたバイト数を返す。
//
// 識別子は ID の生成時点で検証される。ディレクトリ区切りを含む名前は
// 保管先の外を指しうるため、保管先へ渡る前に弾かれる。
func (u *UseCase) Delete(ctx context.Context, backupID string) (int64, error) {
	id, err := backup.NewID(backupID)
	if err != nil {
		return 0, err
	}
	return u.cfg.Store.Delete(ctx, id)
}

// GetRetentionPolicy は現在の保持ポリシーを返す。
func (u *UseCase) GetRetentionPolicy(ctx context.Context) (backup.RetentionPolicy, error) {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return backup.RetentionPolicy{}, err
	}
	return policyFrom(snapshot), nil
}

// SetRetentionPolicy は保持ポリシーを .env へ書き戻す。
//
// 値の検証はドメインの型が行う。keepCount が 0 以下のポリシーは
// そもそも生成できないため、不正な値が .env に書かれる経路がない。
func (u *UseCase) SetRetentionPolicy(
	ctx context.Context, keepCount, keepDays int,
) (backup.RetentionPolicy, error) {
	policy, err := backup.NewRetentionPolicy(keepCount, keepDays)
	if err != nil {
		return backup.RetentionPolicy{}, err
	}

	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return backup.RetentionPolicy{}, err
	}
	next := snapshot.
		With(keyKeep, strconv.Itoa(policy.KeepCount())).
		With(keyKeepDays, strconv.Itoa(policy.KeepDays()))

	if err := u.cfg.Config.Save(ctx, next); err != nil {
		return backup.RetentionPolicy{}, err
	}
	return policy, nil
}

// PruneResult は世代管理の結果。
type PruneResult struct {
	DeletedIDs []backup.ID
	FreedBytes int64
}

// Prune は保持ポリシーを手動で適用する。
// dryRun が真なら削除せず対象だけを返す。
func (u *UseCase) Prune(ctx context.Context, dryRun bool) (PruneResult, error) {
	policy, err := u.GetRetentionPolicy(ctx)
	if err != nil {
		return PruneResult{}, err
	}
	return u.prune(ctx, policy, dryRun)
}

func (u *UseCase) prune(
	ctx context.Context, policy backup.RetentionPolicy, dryRun bool,
) (PruneResult, error) {
	stored, err := u.cfg.Store.List(ctx)
	if err != nil {
		return PruneResult{}, err
	}

	targets := backup.SelectForDeletion(retentionEntries(stored), policy, u.cfg.Clock.Now())
	if dryRun {
		return PruneResult{DeletedIDs: targets, FreedBytes: sizeOf(stored, targets)}, nil
	}

	result := PruneResult{}
	for _, id := range targets {
		freed, err := u.cfg.Store.Delete(ctx, id)
		if err != nil {
			return result, err
		}
		result.DeletedIDs = append(result.DeletedIDs, id)
		result.FreedBytes += freed
	}
	return result, nil
}

// retentionEntries は保持判定の入力を作る。
//
// 日時にはファイルの更新時刻を使う。ファイル名の日時は人が改名できるため、
// 「何を消すか」の判断材料にはしない。
func retentionEntries(stored []port.StoredBackup) []backup.RetentionEntry {
	entries := make([]backup.RetentionEntry, 0, len(stored))
	for _, s := range stored {
		entries = append(entries, backup.RetentionEntry{ID: s.ID, CreatedAt: s.CreatedAt})
	}
	return entries
}

func sizeOf(stored []port.StoredBackup, ids []backup.ID) int64 {
	var total int64
	for _, s := range stored {
		if slices.Contains(ids, s.ID) {
			total += s.SizeBytes
		}
	}
	return total
}
