package rpc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// WorldHandler は WorldService のハンドラ。
type WorldHandler struct {
	mcadminv1connect.UnimplementedWorldServiceHandler

	worlds *worldctl.UseCase
}

// NewWorldHandler は WorldHandler を作る。
func NewWorldHandler(w *worldctl.UseCase) *WorldHandler { return &WorldHandler{worlds: w} }

// ListWorlds はワールドと退避の一覧を返す。
func (h *WorldHandler) ListWorlds(
	ctx context.Context,
	_ *connect.Request[mcadminv1.ListWorldsRequest],
) (*connect.Response[mcadminv1.ListWorldsResponse], error) {
	listing, err := h.worlds.List(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}

	out := &mcadminv1.ListWorldsResponse{
		Worlds:      make([]*mcadminv1.World, len(listing.Worlds)),
		Quarantines: make([]*mcadminv1.Quarantine, len(listing.Quarantines)),
		ActiveLevel: listing.ActiveLevel.String(),
	}
	for i, w := range listing.Worlds {
		out.Worlds[i] = worldToProto(w)
	}
	for i, q := range listing.Quarantines {
		out.Quarantines[i] = quarantineToProto(q)
	}
	return connect.NewResponse(out), nil
}

// SwitchWorld は稼働させるワールドを切り替える。
func (h *WorldHandler) SwitchWorld(
	ctx context.Context,
	req *connect.Request[mcadminv1.SwitchWorldRequest],
) (*connect.Response[mcadminv1.SwitchWorldResponse], error) {
	handle, err := h.worlds.Switch(ctx, req.Msg.GetName())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.SwitchWorldResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// CreateWorld は新しいワールドを作る。
func (h *WorldHandler) CreateWorld(
	ctx context.Context,
	req *connect.Request[mcadminv1.CreateWorldRequest],
) (*connect.Response[mcadminv1.CreateWorldResponse], error) {
	handle, err := h.worlds.Create(ctx, req.Msg.GetName(), req.Msg.GetSeed())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.CreateWorldResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// CloneWorld はワールドを複製する。
func (h *WorldHandler) CloneWorld(
	ctx context.Context,
	req *connect.Request[mcadminv1.CloneWorldRequest],
) (*connect.Response[mcadminv1.CloneWorldResponse], error) {
	handle, err := h.worlds.Clone(ctx, req.Msg.GetSource(), req.Msg.GetDestination())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.CloneWorldResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// RenameWorld はワールドの名前を変える。
func (h *WorldHandler) RenameWorld(
	ctx context.Context,
	req *connect.Request[mcadminv1.RenameWorldRequest],
) (*connect.Response[mcadminv1.RenameWorldResponse], error) {
	handle, err := h.worlds.Rename(ctx, req.Msg.GetFrom(), req.Msg.GetTo())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.RenameWorldResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// DeleteWorld はワールドを削除する。
//
// quarantine_instead は proto の既定値が false になるが、
// 退避を既定にしたいので、明示的に false を送るときだけ即削除する。
// 画面は常にどちらかを明示する。
func (h *WorldHandler) DeleteWorld(
	ctx context.Context,
	req *connect.Request[mcadminv1.DeleteWorldRequest],
) (*connect.Response[mcadminv1.DeleteWorldResponse], error) {
	handle, err := h.worlds.Delete(ctx,
		req.Msg.GetName(), req.Msg.GetConfirmName(), req.Msg.GetQuarantineInstead())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.DeleteWorldResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// PurgeQuarantine は退避されたディレクトリを完全に削除する。
func (h *WorldHandler) PurgeQuarantine(
	ctx context.Context,
	req *connect.Request[mcadminv1.PurgeQuarantineRequest],
) (*connect.Response[mcadminv1.PurgeQuarantineResponse], error) {
	freed, err := h.worlds.PurgeQuarantine(ctx, req.Msg.GetName())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.PurgeQuarantineResponse{FreedBytes: freed}), nil
}

// worldToProto はワールドを転送形式にする。
func worldToProto(w world.World) *mcadminv1.World {
	out := &mcadminv1.World{
		Name:           w.Name().String(),
		Active:         w.IsActive(),
		SizeBytes:      w.SizeBytes(),
		Version:        worldVersionToProto(w.Version()),
		HasSessionLock: w.HasSessionLock(),
	}
	if !w.LastPlayed().IsZero() {
		out.LastPlayed = timestamppb.New(w.LastPlayed())
	}
	return out
}

// quarantineToProto は退避を転送形式にする。
func quarantineToProto(q world.Quarantine) *mcadminv1.Quarantine {
	return &mcadminv1.Quarantine{
		Name:          q.DirName(),
		OriginalLevel: q.OriginalName().String(),
		QuarantinedAt: timestamppb.New(q.QuarantinedAt()),
		SizeBytes:     q.SizeBytes(),
		FromRestore:   q.Kind() == world.QuarantineFromRestore,
	}
}

var _ mcadminv1connect.WorldServiceHandler = (*WorldHandler)(nil)
