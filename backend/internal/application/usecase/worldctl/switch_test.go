package worldctl_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

type harness struct {
	uc      *worldctl.UseCase
	ops     *operations.Manager
	rec     *recorder
	worlds  *fakeWorlds
	config  *fakeConfig
	runtime *fakeRuntime
	con     *fakeConsole
	lock    *fakeLock
}

func (h *harness) console() *fakeConsole { return h.con }

func newHarness(t *testing.T, running bool, names ...string) *harness {
	t.Helper()

	rec := &recorder{}
	state := server.ContainerMissing
	if running {
		state = server.ContainerRunning
	}

	runtime := &fakeRuntime{rec: rec, status: server.ContainerStatus{State: state}}
	console := &fakeConsole{rec: rec}
	worlds := newWorlds(rec, names...)
	config := newConfig(rec, map[string]string{"MC_LEVEL": "world", "MC_VERSION": "26.2"})

	lock := &fakeLock{}
	mgr, err := operations.NewManager(operations.Config{Lock: lock})
	if err != nil {
		t.Fatal(err)
	}

	uc, err := worldctl.New(worldctl.Config{
		Runtime: runtime, Console: console, Worlds: worlds, Config: config,
		Levels: &fakeLevels{version: shared.UnreadableWorldVersion()}, Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &harness{
		uc: uc, ops: mgr, rec: rec, worlds: worlds,
		config: config, runtime: runtime, con: console, lock: lock,
	}
}

func (h *harness) wait(t *testing.T, id operation.ID) operation.Snapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := h.ops.Get(id); ok && snap.State.IsTerminal() {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
	return operation.Snapshot{}
}

func (h *harness) calls() string { return strings.Join(h.rec.list(), ",") }

// 切替は「保存 → 停止 → .env 書き換え → 起動 → 確認」の順。
//
// .env を書き換えてから停止すると、停止の失敗で設定だけが変わった
// 状態になる。停止を先に済ませてから設定を触る。
func TestSwitchCallOrder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	handle, err := h.uc.Switch(context.Background(), "creative")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "SaveAll,Down,WaitStopped,SaveConfig,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
	if got := h.config.get("MC_LEVEL"); got != "creative" {
		t.Errorf("MC_LEVEL が %q", got)
	}
}

// 切替前のワールドは移動も削除もされない。
//
// itzg の LEVEL 方式ではディレクトリが兄弟として並存する。
// 実機でも「data/world が SHA256 単位で無傷」であることを確認済み。
func TestSwitchDoesNotTouchPreviousWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	handle, err := h.uc.Switch(context.Background(), "creative")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if !h.worlds.has("world") {
		t.Error("切替前のワールドが消えている")
	}
	for _, forbidden := range []string{"Quarantine", "Remove", "Rename", "Copy"} {
		if strings.Contains(h.calls(), forbidden) {
			t.Errorf("切替で %s が呼ばれている: %q", forbidden, h.calls())
		}
	}
}

// restart は使わない。.env の変更はコンテナの再作成でしか反映されない。
func TestSwitchUsesUpNotRestart(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")
	handle, _ := h.uc.Switch(context.Background(), "creative")
	h.wait(t, handle.ID())

	if strings.Contains(h.calls(), "Restart") {
		t.Errorf("restart を使っている: %q", h.calls())
	}
}

// 同じワールドへの切替は何もしない。
func TestSwitchToSameWorldIsNoop(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Switch(context.Background(), "world")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	for _, forbidden := range []string{"Down", "Up", "SaveConfig"} {
		if strings.Contains(h.calls(), forbidden) {
			t.Errorf("no-op のはずが %s が呼ばれている: %q", forbidden, h.calls())
		}
	}
}

// 停止中なら保存を省略する。RCON を叩いても失敗するだけ。
func TestSwitchSkipsSaveWhenStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, false, "world", "creative")
	handle, _ := h.uc.Switch(context.Background(), "creative")
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}
	if strings.Contains(h.calls(), "SaveAll") {
		t.Errorf("停止中なのに保存している: %q", h.calls())
	}
}

// 存在しないワールドへの切替は、新規生成として続行する。
// itzg のイメージは LEVEL が未生成なら Paper に作らせる。
func TestSwitchToNewWorldContinues(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Switch(context.Background(), "brandnew")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}
	if h.config.get("MC_LEVEL") != "brandnew" {
		t.Errorf("MC_LEVEL が %q", h.config.get("MC_LEVEL"))
	}
}

// 不正な名前は操作を開始する前に拒否する。
func TestSwitchRejectsInvalidName(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	for _, name := range []string{"../etc", "logs", "", "my world"} {
		if _, err := h.uc.Switch(context.Background(), name); err == nil {
			t.Errorf("%q は拒否されるはず", name)
		}
	}
	if len(h.rec.list()) != 0 {
		t.Errorf("拒否したのに副作用がある: %q", h.calls())
	}
}

// 起動に失敗したら操作は失敗として記録される。
func TestSwitchFailurePropagates(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")
	h.runtime.upErr = errors.New("no configuration file provided")

	handle, err := h.uc.Switch(context.Background(), "creative")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
	if !strings.Contains(snap.ErrorMessage, "no configuration file provided") {
		t.Errorf("失敗理由が %q", snap.ErrorMessage)
	}
}
