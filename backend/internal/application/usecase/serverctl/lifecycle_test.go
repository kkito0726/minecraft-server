package serverctl_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

func newLifecycle(t *testing.T, rt *fakeRuntime, console *fakeConsole) (*serverctl.LifecycleUseCase, *operations.Manager) {
	t.Helper()

	mgr, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}
	uc, err := serverctl.NewLifecycleUseCase(serverctl.LifecycleConfig{
		Runtime:    rt,
		Console:    console,
		Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}
	return uc, mgr
}

func waitDone(t *testing.T, mgr *operations.Manager, id operation.ID) operation.Snapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := mgr.Get(id); ok && snap.State.IsTerminal() {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
	return operation.Snapshot{}
}

// 起動は up してから完了を待つ。restart は使わない。
func TestStartCallOrder(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerMissing}}
	uc, mgr := newLifecycle(t, rt, &fakeConsole{})

	h, err := uc.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, h.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := strings.Join(rt.calls, ","); got != "Up,WaitReady" {
		t.Errorf("呼び出し順が %q。Up,WaitReady のはず", got)
	}
}

// 停止は保存してから down し、停止を待つ。
func TestStopCallOrder(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	console := &fakeConsole{ok: true}
	uc, mgr := newLifecycle(t, rt, console)

	h, err := uc.Stop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, h.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := strings.Join(rt.calls, ","); got != "Down,WaitStopped" {
		t.Errorf("呼び出し順が %q", got)
	}
}

// 再起動は down してから up。restart を呼ばない。
func TestRestartDoesNotUseRestart(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	uc, mgr := newLifecycle(t, rt, &fakeConsole{ok: true})

	h, err := uc.Restart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, h.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	got := strings.Join(rt.calls, ",")
	if got != "Down,WaitStopped,Up,WaitReady" {
		t.Errorf("呼び出し順が %q", got)
	}
	if strings.Contains(got, "Restart") {
		t.Error("restart を使っている。.env の変更が反映されない")
	}
}

// 停止中は保存を省略する。RCON を叩いても失敗するだけ。
func TestStopSkipsSaveWhenStopped(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerMissing}}
	console := &fakeConsole{err: errors.New("呼ばれるべきでない")}
	uc, mgr := newLifecycle(t, rt, console)

	h, err := uc.Stop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, h.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}
}

// 実行中は次の操作を拒否する。
func TestLifecycleRespectsExclusiveLock(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	uc, mgr := newLifecycle(t, rt, &fakeConsole{ok: true})

	h, err := uc.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, mgr, h.ID())

	// 完了後は次が通ること
	if _, err := uc.Stop(context.Background()); err != nil {
		t.Errorf("完了後に開始できない: %v", err)
	}
}

// docker の失敗は操作の失敗として記録される。
func TestFailurePropagatesToOperation(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{
		status: server.ContainerStatus{State: server.ContainerMissing},
		err:    errors.New("no configuration file provided"),
	}
	uc, mgr := newLifecycle(t, rt, &fakeConsole{})

	h, err := uc.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, h.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v。失敗のはず", snap.State)
	}
	if !strings.Contains(snap.ErrorMessage, "no configuration file provided") {
		t.Errorf("失敗理由が %q", snap.ErrorMessage)
	}
}

func TestNewLifecycleUseCaseValidates(t *testing.T) {
	t.Parallel()

	if _, err := serverctl.NewLifecycleUseCase(serverctl.LifecycleConfig{}); err == nil {
		t.Error("エラーになるはず")
	}
}
