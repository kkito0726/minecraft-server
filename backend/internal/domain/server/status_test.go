package server_test

import (
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
)

// RCON を伴う手順を行ってよいかは IsUp で判断する。
// 停止中に save-off を送っても失敗するだけなので、事前に分岐する。
func TestContainerStateIsUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state server.ContainerState
		want  bool
	}{
		{server.ContainerRunning, true},
		{server.ContainerExited, false},
		{server.ContainerRestarting, false},
		{server.ContainerMissing, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.state.IsUp(); got != tt.want {
				t.Errorf("IsUp() = %v。%v のはず", got, tt.want)
			}
		})
	}
}

// 状態の名前はそのまま画面に出る。空にならないこと。
func TestContainerStateString(t *testing.T) {
	t.Parallel()

	states := []server.ContainerState{
		server.ContainerRunning,
		server.ContainerExited,
		server.ContainerRestarting,
		server.ContainerMissing,
		server.ContainerState(99),
	}

	for _, s := range states {
		if s.String() == "" {
			t.Errorf("%d の String() が空", s)
		}
	}
}

func TestSavingStateString(t *testing.T) {
	t.Parallel()

	if got := server.SavingAssumedOn.String(); got != "有効（推定）" {
		t.Errorf("String() が %q", got)
	}
	if got := server.SavingSuspectOff.String(); got != "停止している可能性あり" {
		t.Errorf("String() が %q", got)
	}
}
