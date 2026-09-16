package rpc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
)

// ServerHandler は ServerService のハンドラ。
type ServerHandler struct {
	mcadminv1connect.UnimplementedServerServiceHandler

	status     *serverctl.StatusUseCase
	lifecycle  *serverctl.LifecycleUseCase
	settings   *serverctl.SettingsUseCase
	operations *operations.Manager
	// configKeys は API から返してよい .env のキー。
	//
	// RCON_PASSWORD や ADMIN_TOKEN を含めない。許可リスト方式にするのは、
	// 新しいキーが増えたときに既定で秘匿されるようにするため。
	configKeys []string
}

// NewServerHandler は ServerHandler を作る。
func NewServerHandler(
	status *serverctl.StatusUseCase,
	lifecycle *serverctl.LifecycleUseCase,
	gameSettings *serverctl.SettingsUseCase,
	ops *operations.Manager,
	configKeys []string,
) *ServerHandler {
	return &ServerHandler{
		status:     status,
		lifecycle:  lifecycle,
		settings:   gameSettings,
		operations: ops,
		configKeys: configKeys,
	}
}

// GetStatus は現在の状態を返す。
func (h *ServerHandler) GetStatus(
	ctx context.Context,
	_ *connect.Request[mcadminv1.GetStatusRequest],
) (*connect.Response[mcadminv1.GetStatusResponse], error) {
	s, err := h.status.Execute(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}

	out := &mcadminv1.GetStatusResponse{
		ContainerState:               containerStateToProto(s.Container.State),
		Healthy:                      s.Container.Healthy,
		ConfiguredVersion:            s.ConfiguredVersion,
		ActiveLevel:                  s.ActiveLevel.String(),
		ActiveWorldVersion:           worldVersionToProto(s.ActiveWorldVersion),
		OnlinePlayers:                int32(s.OnlinePlayers),
		MaxPlayers:                   int32(s.MaxPlayers),
		SavingState:                  savingStateToProto(s.SavingState),
		InterruptedOperationDetected: s.InterruptedDetected,
	}
	if !s.Container.StartedAt.IsZero() {
		out.ContainerStartedAt = timestamppb.New(s.Container.StartedAt)
	}
	if active, ok := h.operations.Active(); ok {
		out.ActiveOperation = operationToProto(active)
	}
	return connect.NewResponse(out), nil
}

// StartServer はサーバーを起動する。
func (h *ServerHandler) StartServer(
	ctx context.Context,
	_ *connect.Request[mcadminv1.StartServerRequest],
) (*connect.Response[mcadminv1.StartServerResponse], error) {
	handle, err := h.lifecycle.Start(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.StartServerResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// StopServer はサーバーを停止する。
func (h *ServerHandler) StopServer(
	ctx context.Context,
	_ *connect.Request[mcadminv1.StopServerRequest],
) (*connect.Response[mcadminv1.StopServerResponse], error) {
	handle, err := h.lifecycle.Stop(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.StopServerResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// RestartServer はサーバーを再起動する。
func (h *ServerHandler) RestartServer(
	ctx context.Context,
	_ *connect.Request[mcadminv1.RestartServerRequest],
) (*connect.Response[mcadminv1.RestartServerResponse], error) {
	handle, err := h.lifecycle.Restart(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.RestartServerResponse{
		Operation: operationToProto(handle.Snapshot()),
	}), nil
}

// GetGameSettings は .env のゲーム設定を返す。
func (h *ServerHandler) GetGameSettings(
	ctx context.Context,
	_ *connect.Request[mcadminv1.GetGameSettingsRequest],
) (*connect.Response[mcadminv1.GetGameSettingsResponse], error) {
	reading, err := h.settings.Get(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&mcadminv1.GetGameSettingsResponse{
		Settings: gameSettingsToProto(reading.Settings),
		Warnings: reading.Warnings,
	}), nil
}

// UpdateGameSettings はゲーム設定を .env に書き、求められれば反映する。
func (h *ServerHandler) UpdateGameSettings(
	ctx context.Context,
	req *connect.Request[mcadminv1.UpdateGameSettingsRequest],
) (*connect.Response[mcadminv1.UpdateGameSettingsResponse], error) {
	s, err := gameSettingsFromProto(req.Msg.GetSettings())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &mcadminv1.UpdateGameSettingsResponse{Settings: gameSettingsToProto(s)}

	if !req.Msg.GetApplyNow() {
		if err := h.settings.Save(ctx, s); err != nil {
			return nil, toConnectError(err)
		}
		return connect.NewResponse(out), nil
	}

	result, err := h.settings.SaveAndApply(ctx, s)
	if err != nil {
		return nil, toConnectError(err)
	}
	if result.Started {
		out.Operation = operationToProto(result.Handle.Snapshot())
	}
	return connect.NewResponse(out), nil
}

var _ mcadminv1connect.ServerServiceHandler = (*ServerHandler)(nil)
