package upload_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// 取り込みはワールドにもサーバーにも触らない。
// ここが呼ばれたら設計が崩れているので、すべて no-op で十分。

type stubRuntime struct{}

func (stubRuntime) Up(context.Context, port.LogSink) error   { return nil }
func (stubRuntime) Down(context.Context, port.LogSink) error { return nil }
func (stubRuntime) Exec(context.Context, []string) (string, error) {
	return "", nil
}
func (stubRuntime) Status(context.Context) (server.ContainerStatus, error) {
	return server.ContainerStatus{State: server.ContainerMissing}, nil
}
func (stubRuntime) WaitReady(context.Context, time.Duration) error   { return nil }
func (stubRuntime) WaitStopped(context.Context, time.Duration) error { return nil }

type stubConsole struct{}

func (stubConsole) SaveOff(context.Context) error { return nil }
func (stubConsole) SaveAll(context.Context) error { return nil }
func (stubConsole) SaveOn(context.Context) error  { return nil }
func (stubConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return 0, 0, false, nil
}

type stubWorlds struct{ available int64 }

func (s stubWorlds) AvailableBytes(context.Context) (int64, error) { return s.available, nil }

func (stubWorlds) List(context.Context, world.Name) ([]world.World, error) { return nil, nil }
func (stubWorlds) Exists(context.Context, world.Name) (bool, error)        { return false, nil }
func (stubWorlds) Copy(context.Context, world.Name, world.Name, port.Progress) error {
	return nil
}
func (stubWorlds) Rename(context.Context, world.Name, world.Name) error { return nil }
func (stubWorlds) Quarantine(
	context.Context, world.Name, world.QuarantineKind,
) (world.Quarantine, error) {
	return world.Quarantine{}, nil
}
func (stubWorlds) Restore(context.Context, world.Quarantine, world.Name) error { return nil }
func (stubWorlds) Remove(context.Context, world.Name) error                    { return nil }
func (stubWorlds) ListQuarantines(context.Context) ([]world.Quarantine, error) {
	return nil, nil
}
func (stubWorlds) RemoveQuarantine(context.Context, world.Quarantine) (int64, error) {
	return 0, nil
}

type stubConfig struct{}

func (stubConfig) Load(context.Context) (port.ConfigSnapshot, error) {
	return stubSnapshot{}, nil
}
func (stubConfig) Save(context.Context, port.ConfigSnapshot) error { return nil }

type stubSnapshot struct{}

func (stubSnapshot) Get(key string) (string, bool) {
	switch key {
	case "MC_LEVEL":
		return "world", true
	case "MC_VERSION":
		return "26.2", true
	}
	return "", false
}
func (s stubSnapshot) With(string, string) port.ConfigSnapshot { return s }
func (stubSnapshot) Keys() []string                            { return nil }

type stubLevels struct{}

func (stubLevels) ReadWorld(context.Context, world.Name) shared.WorldVersion {
	return shared.UnreadableWorldVersion()
}

// Read は level.dat を読めないものとして扱う。取り込みが版に
// 依存しないこと（読めなくても止まらないこと）を確かめたい。
func (stubLevels) Read(context.Context, io.Reader) shared.WorldVersion {
	return shared.UnreadableWorldVersion()
}

// assertEmpty は保管先に何も残っていないことを確かめる。
//
// 拒否したのにファイルが残ると、次の一覧で壊れたアーカイブが見える。
func assertEmpty(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, filepath.Base(e.Name()))
	}
	t.Errorf("拒否したのに保管先に残っている: %v", names)
}
