package rpc_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
)

func validSettings() *mcadminv1.GameSettings {
	return &mcadminv1.GameSettings{
		Difficulty: mcadminv1.Difficulty_DIFFICULTY_HARD,
		Mode:       mcadminv1.GameMode_GAME_MODE_CREATIVE,
		Motd:       "§aようこそ",
		MaxPlayers: 8, ViewDistance: 9, SimulationDistance: 6,
	}
}

// .env にキーが無ければ compose の既定値を返す。
func TestGetGameSettingsReturnsDefaults(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.GetGameSettings(context.Background(),
		connect.NewRequest(&mcadminv1.GetGameSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	s := res.Msg.GetSettings()
	if s.GetDifficulty() != mcadminv1.Difficulty_DIFFICULTY_NORMAL || s.GetMaxPlayers() != 5 ||
		s.GetViewDistance() != 7 || s.GetSimulationDistance() != 5 {
		t.Errorf("既定値と違う: %+v", s)
	}
}

// 規則外の値は利用者が直せるので、伏せずに InvalidArgument で返す。
func TestUpdateGameSettingsRejectsInvalid(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	bad := validSettings()
	bad.SimulationDistance = 20

	_, err := h.serverClient.UpdateGameSettings(context.Background(),
		connect.NewRequest(&mcadminv1.UpdateGameSettingsRequest{Settings: bad}))

	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeInvalidArgument {
		t.Fatalf("InvalidArgument を期待したが %v", err)
	}
}

// 保存だけなら操作は始まらない。
func TestUpdateGameSettingsSaveOnlyStartsNoOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.UpdateGameSettings(context.Background(),
		connect.NewRequest(&mcadminv1.UpdateGameSettingsRequest{Settings: validSettings()}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetOperation() != nil {
		t.Error("保存だけなのに操作が始まった")
	}
	if res.Msg.GetSettings().GetDifficulty() != mcadminv1.Difficulty_DIFFICULTY_HARD {
		t.Errorf("書き込んだ設定が返っていない: %+v", res.Msg.GetSettings())
	}
}

// 稼働中に今すぐ反映すると、作り直しの操作を返す。
func TestUpdateGameSettingsApplyNowStartsOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.UpdateGameSettings(context.Background(),
		connect.NewRequest(&mcadminv1.UpdateGameSettingsRequest{Settings: validSettings(), ApplyNow: true}))
	if err != nil {
		t.Fatal(err)
	}
	op := res.Msg.GetOperation()
	if op == nil {
		t.Fatal("稼働中なのに操作が返っていない")
	}
	if op.GetKind() != mcadminv1.OperationKind_OPERATION_KIND_SERVER_APPLY_SETTINGS {
		t.Errorf("操作の種類が %v", op.GetKind())
	}
}

// 停止中は勝手に起動しない。操作は返らない。
func TestUpdateGameSettingsApplyNowWhenStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.runtime.status = server.ContainerStatus{State: server.ContainerMissing}

	res, err := h.serverClient.UpdateGameSettings(context.Background(),
		connect.NewRequest(&mcadminv1.UpdateGameSettingsRequest{Settings: validSettings(), ApplyNow: true}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetOperation() != nil {
		t.Error("停止中なのに作り直しを始めた")
	}
}
