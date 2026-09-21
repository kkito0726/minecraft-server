package serverctl_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

// settingsSnapshot は .env の写し。With は新しい写しを返し、元を変えない。
type settingsSnapshot struct{ values map[string]string }

func (s *settingsSnapshot) Get(k string) (string, bool) { v, ok := s.values[k]; return v, ok }

func (s *settingsSnapshot) With(k, v string) port.ConfigSnapshot {
	next := make(map[string]string, len(s.values)+1)
	for key, val := range s.values {
		next[key] = val
	}
	next[k] = v
	return &settingsSnapshot{values: next}
}

// settingsConfig は保存された内容と、保存した時点を記録する。
// 呼び出し順を確かめるため、記録は runtime と同じ列に積む。
type settingsConfig struct {
	current *settingsSnapshot
	saved   *settingsSnapshot
	rt      *fakeRuntime
	saveErr error
}

func (c *settingsConfig) Load(context.Context) (port.ConfigSnapshot, error) { return c.current, nil }

func (c *settingsConfig) Save(_ context.Context, s port.ConfigSnapshot) error {
	if c.rt != nil {
		c.rt.calls = append(c.rt.calls, "SaveEnv")
	}
	if c.saveErr != nil {
		return c.saveErr
	}
	c.saved = s.(*settingsSnapshot)
	return nil
}

func newSettings(
	t *testing.T, rt *fakeRuntime, cfg *settingsConfig,
) (*serverctl.SettingsUseCase, *operations.Manager) {
	t.Helper()

	mgr, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := serverctl.NewLifecycleUseCase(serverctl.LifecycleConfig{
		Runtime: rt, Console: &fakeConsole{ok: true}, Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}
	uc, err := serverctl.NewSettingsUseCase(serverctl.SettingsConfig{
		Config: cfg, Runtime: rt, Lifecycle: lifecycle, Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}
	return uc, mgr
}

func envWith(values map[string]string) *settingsSnapshot {
	return &settingsSnapshot{values: values}
}

func mustSettings(t *testing.T) settings.GameSettings {
	t.Helper()

	s, err := settings.New(settings.Params{
		Difficulty: settings.DifficultyHard, Mode: settings.GameModeCreative,
		MOTD: "§aようこそ", MaxPlayers: 8, ViewDistance: 9, SimulationDistance: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSettingsGetReadsEnv(t *testing.T) {
	t.Parallel()

	cfg := &settingsConfig{current: envWith(map[string]string{
		"MC_DIFFICULTY": "HARD", "MC_MOTD": "§aE2E のサーバー",
		"MC_MAX_PLAYERS": "10", "MC_VIEW_DISTANCE": "9", "MC_SIMULATION_DISTANCE": "6",
		"MC_MODE": "CREATIVE", "MC_HARDCORE": "true",
	})}
	uc, _ := newSettings(t, &fakeRuntime{}, cfg)

	got, err := uc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := got.Settings
	if s.Difficulty() != settings.DifficultyHard || s.MOTD() != "§aE2E のサーバー" ||
		s.MaxPlayers() != 10 || s.ViewDistance() != 9 || s.SimulationDistance() != 6 {
		t.Errorf("読み取りが違う: %+v", s)
	}
	// 大文字でも読む。手で書かれた .env を弾くと設定画面が開けなくなる。
	if s.Mode() != settings.GameModeCreative {
		t.Errorf("モードが %q", s.Mode())
	}
	if !s.Hardcore() {
		t.Error("ハードコアを読めていない")
	}
	if len(got.Warnings) != 0 {
		t.Errorf("警告は出ないはず: %v", got.Warnings)
	}
}

// キーが無ければ compose の既定値で補う。これは異常ではないので警告しない。
func TestSettingsGetFallsBackToDefaultsSilently(t *testing.T) {
	t.Parallel()

	uc, _ := newSettings(t, &fakeRuntime{}, &settingsConfig{current: envWith(map[string]string{
		"MC_MOTD": "",
	})})

	got, err := uc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings != settings.Defaults() {
		t.Errorf("既定値にならない: %+v", got.Settings)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("未設定は警告しないはず: %v", got.Warnings)
	}
}

// 手で書かれた読めない値があっても画面は開ける。何を補ったかは警告で伝える。
func TestSettingsGetWarnsOnUnreadableValues(t *testing.T) {
	t.Parallel()

	uc, _ := newSettings(t, &fakeRuntime{}, &settingsConfig{current: envWith(map[string]string{
		"MC_DIFFICULTY": "hardcore", "MC_MAX_PLAYERS": "many",
		"MC_VIEW_DISTANCE": "64", "MC_SIMULATION_DISTANCE": "4",
	})})

	got, err := uc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d := settings.Defaults()
	if got.Settings.Difficulty() != d.Difficulty() || got.Settings.MaxPlayers() != d.MaxPlayers() ||
		got.Settings.ViewDistance() != d.ViewDistance() {
		t.Errorf("読めない値が既定に戻っていない: %+v", got.Settings)
	}
	joined := strings.Join(got.Warnings, "\n")
	for _, key := range []string{"MC_DIFFICULTY", "MC_MAX_PLAYERS", "MC_VIEW_DISTANCE"} {
		if !strings.Contains(joined, key) {
			t.Errorf("%s の警告が無い: %v", key, got.Warnings)
		}
	}
}

func TestSettingsGetSettlesInvalidCombination(t *testing.T) {
	t.Parallel()

	uc, _ := newSettings(t, &fakeRuntime{}, &settingsConfig{current: envWith(map[string]string{
		"MC_VIEW_DISTANCE": "6", "MC_SIMULATION_DISTANCE": "10", "MC_MOTD": `bad "quote"`,
	})})

	got, err := uc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.SimulationDistance() != 6 {
		t.Errorf("シミュレーション距離が描画距離に揃っていない: %d", got.Settings.SimulationDistance())
	}
	if got.Settings.MOTD() != settings.Defaults().MOTD() {
		t.Errorf("規則外の MOTD が既定に戻っていない: %q", got.Settings.MOTD())
	}
	if len(got.Warnings) < 2 {
		t.Errorf("直したことが伝わっていない: %v", got.Warnings)
	}
}

func TestSettingsSaveWritesAllKeys(t *testing.T) {
	t.Parallel()

	cfg := &settingsConfig{current: envWith(map[string]string{"MC_LEVEL": "world"})}
	uc, _ := newSettings(t, &fakeRuntime{}, cfg)

	if err := uc.Save(context.Background(), mustSettings(t)); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"MC_LEVEL": "world", "MC_DIFFICULTY": "hard", "MC_MOTD": "§aようこそ",
		"MC_MAX_PLAYERS": "8", "MC_VIEW_DISTANCE": "9", "MC_SIMULATION_DISTANCE": "6",
		"MC_MODE": "creative",
	}
	for k, v := range want {
		if got, _ := cfg.saved.Get(k); got != v {
			t.Errorf("%s = %q。%q のはず", k, got, v)
		}
	}
}

// 他の操作の最中に .env を書くと、相手の書き込みを競合で失敗させる。
func TestSettingsSaveRefusesWhileBusy(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	cfg := &settingsConfig{current: envWith(map[string]string{})}
	uc, mgr := newSettings(t, rt, cfg)

	release := make(chan struct{})
	blocker, err := mgr.Start(context.Background(), operation.KindBackupCreate, []string{"待つ"},
		func(context.Context, operations.Reporter) error {
			<-release
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(release)
		waitDone(t, mgr, blocker.ID())
	}()

	err = uc.Save(context.Background(), mustSettings(t))
	if !errors.Is(err, operations.ErrBusy) {
		t.Fatalf("ErrBusy を期待したが %v", err)
	}
	if cfg.saved != nil {
		t.Error("実行中に .env を書いてしまった")
	}
}

// 止まっているサーバーは勝手に起動しない。書いておけば次の起動で反映される。
func TestSettingsApplyWhenStoppedOnlyWrites(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerMissing}}
	cfg := &settingsConfig{current: envWith(map[string]string{}), rt: rt}
	uc, _ := newSettings(t, rt, cfg)

	result, err := uc.SaveAndApply(context.Background(), mustSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Started {
		t.Error("停止中なのに作り直しを始めた")
	}
	if got := strings.Join(rt.calls, ","); got != "SaveEnv" {
		t.Errorf("呼び出しが %q。書き込みだけのはず", got)
	}
}

// 稼働中は、書いてから再起動と同じ手順で作り直す。restart は使わない。
func TestSettingsApplyWhenRunningRecreates(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	cfg := &settingsConfig{current: envWith(map[string]string{}), rt: rt}
	uc, mgr := newSettings(t, rt, cfg)

	result, err := uc.SaveAndApply(context.Background(), mustSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Started {
		t.Fatal("稼働中なのに作り直しを始めていない")
	}
	snap := waitDone(t, mgr, result.Handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}
	if snap.Kind != operation.KindServerApplySettings {
		t.Errorf("操作の種類が %v", snap.Kind)
	}
	if got := strings.Join(rt.calls, ","); got != "SaveEnv,Down,WaitStopped,Up,WaitReady" {
		t.Errorf("呼び出し順が %q", got)
	}
}

// 書き込みに失敗したら、まだ誰も切断していない段階で止める。
func TestSettingsApplyStopsBeforeDisruptingOnWriteFailure(t *testing.T) {
	t.Parallel()

	rt := &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}}
	cfg := &settingsConfig{current: envWith(map[string]string{}), rt: rt, saveErr: errors.New("競合")}
	uc, mgr := newSettings(t, rt, cfg)

	result, err := uc.SaveAndApply(context.Background(), mustSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	snap := waitDone(t, mgr, result.Handle.ID())
	if snap.State != operation.StateFailed {
		t.Fatal("書き込みに失敗したのに成功扱いになった")
	}
	if slices.Contains(rt.calls, "Down") {
		t.Errorf("書き込みに失敗したのに停止した: %v", rt.calls)
	}
}

func TestNewSettingsUseCaseRequiresDependencies(t *testing.T) {
	t.Parallel()

	if _, err := serverctl.NewSettingsUseCase(serverctl.SettingsConfig{}); err == nil {
		t.Error("依存が無いのに作れてしまった")
	}
}

// ハードコアは設定の保存で書き換えない。
//
// 画面から変えられない値なので、読んだ結果を書き戻すと、読めなかった値を
// 既定に倒した結果がそのまま .env に焼き付く。決めるのは作成時だけ。
func TestSettingsSaveDoesNotTouchHardcore(t *testing.T) {
	t.Parallel()

	cfg := &settingsConfig{current: envWith(map[string]string{
		"MC_HARDCORE": "TRUE", "MC_LEVEL": "world",
	})}
	uc, _ := newSettings(t, &fakeRuntime{}, cfg)

	if err := uc.Save(context.Background(), mustSettings(t)); err != nil {
		t.Fatal(err)
	}

	if got, _ := cfg.saved.Get("MC_HARDCORE"); got != "TRUE" {
		t.Errorf("MC_HARDCORE が %q。触らないはず", got)
	}
}

// 読めないモードは既定に倒し、そのことを知らせる。
func TestSettingsGetWarnsOnUnreadableMode(t *testing.T) {
	t.Parallel()

	uc, _ := newSettings(t, &fakeRuntime{}, &settingsConfig{current: envWith(map[string]string{
		"MC_MODE": "god",
	})})

	got, err := uc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Mode() != settings.GameModeSurvival {
		t.Errorf("既定に倒れていない: %q", got.Settings.Mode())
	}
	if !slices.ContainsFunc(got.Warnings, func(w string) bool {
		return strings.Contains(w, "MC_MODE")
	}) {
		t.Errorf("MC_MODE の警告が無い: %v", got.Warnings)
	}
}
