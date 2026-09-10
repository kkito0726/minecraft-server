package serverctl_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// fakeRuntime は呼び出しを記録する ContainerRuntime。
type fakeRuntime struct {
	calls  []string
	status server.ContainerStatus
	err    error
}

func (f *fakeRuntime) Up(context.Context, port.LogSink) error {
	f.calls = append(f.calls, "Up")
	return f.err
}

func (f *fakeRuntime) Down(context.Context, port.LogSink) error {
	f.calls = append(f.calls, "Down")
	return f.err
}

func (f *fakeRuntime) Status(context.Context) (server.ContainerStatus, error) {
	return f.status, f.err
}

func (f *fakeRuntime) WaitStopped(context.Context, time.Duration) error {
	f.calls = append(f.calls, "WaitStopped")
	return f.err
}

func (f *fakeRuntime) WaitReady(context.Context, time.Duration) error {
	f.calls = append(f.calls, "WaitReady")
	return f.err
}

// fakeConsole は人数を返す ServerConsole。
type fakeConsole struct {
	online, max int
	ok          bool
	err         error
	saveOnCalls int
}

func (f *fakeConsole) SaveOff(context.Context) error { return nil }
func (f *fakeConsole) SaveAll(context.Context) error { return nil }

func (f *fakeConsole) SaveOn(context.Context) error {
	f.saveOnCalls++
	return nil
}

func (f *fakeConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return f.online, f.max, f.ok, f.err
}

// fakeConfig は .env の代わり。
type fakeConfig struct{ values map[string]string }

func (f *fakeConfig) Get(key string) (string, bool) {
	v, ok := f.values[key]
	return v, ok
}

func (f *fakeConfig) With(key, value string) port.ConfigSnapshot {
	next := map[string]string{}
	for k, v := range f.values {
		next[k] = v
	}
	next[key] = value
	return &fakeConfig{values: next}
}

type fakeConfigStore struct{ snapshot *fakeConfig }

func (f *fakeConfigStore) Load(context.Context) (port.ConfigSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeConfigStore) Save(context.Context, port.ConfigSnapshot) error { return nil }

// fakeLevels は level.dat の読み取り。
type fakeLevels struct{ version shared.WorldVersion }

func (f *fakeLevels) ReadWorld(context.Context, world.Name) shared.WorldVersion {
	return f.version
}

func (f *fakeLevels) Read(context.Context, io.Reader) shared.WorldVersion { return f.version }

func newStatusUseCase(t *testing.T, rt *fakeRuntime, console *fakeConsole) *serverctl.StatusUseCase {
	t.Helper()

	dv, _ := shared.NewDataVersion(4903)
	uc, err := serverctl.NewStatusUseCase(serverctl.StatusConfig{
		Runtime: rt,
		Console: console,
		Config: &fakeConfigStore{snapshot: &fakeConfig{values: map[string]string{
			"MC_VERSION": "26.2",
			"MC_LEVEL":   "world",
		}}},
		Levels: &fakeLevels{version: shared.NewWorldVersion("26.2", dv, false, "world")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return uc
}

func TestStatusCollectsEverything(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	rt := &fakeRuntime{status: server.ContainerStatus{
		State:     server.ContainerRunning,
		Healthy:   true,
		StartedAt: started,
		Image:     "itzg/minecraft-server:latest",
	}}
	console := &fakeConsole{online: 2, max: 5, ok: true}

	got, err := newStatusUseCase(t, rt, console).Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute に失敗: %v", err)
	}

	if got.Container.State != server.ContainerRunning || !got.Container.Healthy {
		t.Errorf("コンテナ状態が %+v", got.Container)
	}
	if got.ConfiguredVersion != "26.2" {
		t.Errorf("ConfiguredVersion が %q", got.ConfiguredVersion)
	}
	if got.ActiveLevel.String() != "world" {
		t.Errorf("ActiveLevel が %q", got.ActiveLevel)
	}
	if !got.ActiveWorldVersion.Readable() {
		t.Error("ワールドのバージョンが読めていない")
	}
	if got.OnlinePlayers != 2 || got.MaxPlayers != 5 {
		t.Errorf("人数が %d/%d", got.OnlinePlayers, got.MaxPlayers)
	}
}

// 人数を解釈できなければ -1 を返す。状態取得そのものは成功させる。
func TestStatusWithUnparsablePlayerCount(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	console := &fakeConsole{ok: false}

	got, err := newStatusUseCase(t, rt, console).Execute(context.Background())
	if err != nil {
		t.Fatalf("エラーにせず不明として扱うはず: %v", err)
	}
	if got.OnlinePlayers != -1 || got.MaxPlayers != -1 {
		t.Errorf("人数が %d/%d。不明なら -1 のはず", got.OnlinePlayers, got.MaxPlayers)
	}
}

// コンテナが停止していれば RCON を叩かない。叩いても失敗するだけ。
func TestStatusSkipsRCONWhenStopped(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerMissing}}
	console := &fakeConsole{err: errors.New("呼ばれるべきでない")}

	got, err := newStatusUseCase(t, rt, console).Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute に失敗: %v", err)
	}
	if got.OnlinePlayers != -1 {
		t.Errorf("停止中なのに人数が %d", got.OnlinePlayers)
	}
}

// RCON が失敗しても状態取得は成功させる。
// 人数が読めないことで画面全体が使えなくなってはいけない。
func TestStatusToleratesRCONFailure(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	console := &fakeConsole{err: errors.New("rcon-cli に失敗")}

	got, err := newStatusUseCase(t, rt, console).Execute(context.Background())
	if err != nil {
		t.Fatalf("RCON の失敗で状態取得を失敗させないはず: %v", err)
	}
	if got.OnlinePlayers != -1 {
		t.Errorf("人数が %d", got.OnlinePlayers)
	}
}

// 中断が検出されていれば、保存が止まっている可能性を示す。
func TestStatusReflectsInterruption(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	uc := newStatusUseCase(t, rt, &fakeConsole{ok: true})

	uc.SetInterrupted(true)
	got, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.InterruptedDetected {
		t.Error("中断が反映されていない")
	}
	if got.SavingState != server.SavingSuspectOff {
		t.Errorf("保存状態が %v。停止の可能性ありのはず", got.SavingState)
	}

	uc.SetInterrupted(false)
	got, _ = uc.Execute(context.Background())
	if got.SavingState != server.SavingAssumedOn {
		t.Errorf("保存状態が %v", got.SavingState)
	}
}

func TestNewStatusUseCaseValidates(t *testing.T) {
	t.Parallel()

	if _, err := serverctl.NewStatusUseCase(serverctl.StatusConfig{}); err == nil {
		t.Error("エラーになるはず")
	}
}
