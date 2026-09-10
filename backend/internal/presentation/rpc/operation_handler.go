package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// OperationHandler は OperationService のハンドラ。
type OperationHandler struct {
	mcadminv1connect.UnimplementedOperationServiceHandler

	manager *operations.Manager
}

// NewOperationHandler は OperationHandler を作る。
func NewOperationHandler(m *operations.Manager) *OperationHandler {
	return &OperationHandler{manager: m}
}

// GetOperation は識別子で操作を取得する。
func (h *OperationHandler) GetOperation(
	_ context.Context,
	req *connect.Request[mcadminv1.GetOperationRequest],
) (*connect.Response[mcadminv1.GetOperationResponse], error) {
	id, err := operation.NewID(req.Msg.GetOperationId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	snap, ok := h.manager.Get(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound,
			errors.New("操作が見つかりません"))
	}
	return connect.NewResponse(&mcadminv1.GetOperationResponse{
		Operation: operationToProto(snap),
	}), nil
}

// GetActiveOperation は進行中の操作を返す。
//
// 識別子を保持していないクライアント（別端末、localStorage を消した場合）が
// 現在の状況を知るための入口。
func (h *OperationHandler) GetActiveOperation(
	context.Context,
	*connect.Request[mcadminv1.GetActiveOperationRequest],
) (*connect.Response[mcadminv1.GetActiveOperationResponse], error) {
	snap, ok := h.manager.Active()
	out := &mcadminv1.GetActiveOperationResponse{Present: ok}
	if ok {
		out.Operation = operationToProto(snap)
	}
	return connect.NewResponse(out), nil
}

// ListOperations は直近の操作を返す。
func (h *OperationHandler) ListOperations(
	_ context.Context,
	req *connect.Request[mcadminv1.ListOperationsRequest],
) (*connect.Response[mcadminv1.ListOperationsResponse], error) {
	list := h.manager.List(int(req.Msg.GetLimit()))

	out := &mcadminv1.ListOperationsResponse{
		Operations: make([]*mcadminv1.Operation, len(list)),
	}
	for i, s := range list {
		out.Operations[i] = operationToProto(s)
	}
	return connect.NewResponse(out), nil
}

// WatchOperation は進捗を逐次配信する。
//
// from_seq 以降の記録済みイベントを再送してから追従する。
// これが無いと、ブラウザを再読み込みした直後の画面がスピナーだけになる。
func (h *OperationHandler) WatchOperation(
	ctx context.Context,
	req *connect.Request[mcadminv1.WatchOperationRequest],
	stream *connect.ServerStream[mcadminv1.WatchOperationResponse],
) error {
	id, err := operation.NewID(req.Msg.GetOperationId())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	events, unsubscribe, err := h.manager.Subscribe(ctx, id, req.Msg.GetFromSeq())
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		case e, open := <-events:
			if !open {
				return nil
			}
			if err := stream.Send(operationEventToProto(e)); err != nil {
				// クライアントが切断した。異常ではない。
				return nil
			}
		}
	}
}

var _ mcadminv1connect.OperationServiceHandler = (*OperationHandler)(nil)
