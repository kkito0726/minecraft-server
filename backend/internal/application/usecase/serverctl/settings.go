package serverctl

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
)

// ゲーム設定の .env のキー。
//
// compose.yaml がこれらをイメージの環境変数へ渡し、OVERRIDE_SERVER_PROPERTIES=TRUE
// によって起動のたびに server.properties へ書かれる。.env が常に正になる。
const (
	keyDifficulty         = "MC_DIFFICULTY"
	keyMode               = "MC_MODE"
	keyHardcore           = "MC_HARDCORE"
	keyMOTD               = "MC_MOTD"
	keyMaxPlayers         = "MC_MAX_PLAYERS"
	keyViewDistance       = "MC_VIEW_DISTANCE"
	keySimulationDistance = "MC_SIMULATION_DISTANCE"
)

// SettingsConfig はゲーム設定のユースケースの依存。
type SettingsConfig struct {
	Config    port.ServerConfig
	Runtime   port.ContainerRuntime
	Lifecycle *LifecycleUseCase
	// Operations は反映（作り直し）を排他に走らせるためと、
	// 保存だけのときに他の操作の最中かを確かめるために使う。
	Operations *operations.Manager
}

// SettingsUseCase はゲーム設定の読み書きと反映。
type SettingsUseCase struct {
	cfg SettingsConfig
}

// NewSettingsUseCase は SettingsUseCase を作る。
func NewSettingsUseCase(cfg SettingsConfig) (*SettingsUseCase, error) {
	if cfg.Config == nil || cfg.Runtime == nil || cfg.Lifecycle == nil || cfg.Operations == nil {
		return nil, errors.New("config / runtime / lifecycle / operations が必要です")
	}
	return &SettingsUseCase{cfg: cfg}, nil
}

// SettingsReading は .env から読んだゲーム設定。
type SettingsReading struct {
	Settings settings.GameSettings
	// Warnings は読めなかった値の説明。既定値で補って表示していることを伝える。
	Warnings []string
}

// Get は .env のゲーム設定を読む。
//
// 手で書かれた .env に読めない値があっても、画面は開けるようにする。
// エラーにすると、設定を直すための画面そのものが開けなくなる。
// 読めなかったキーは既定値で補い、その事実を Warnings で伝える。
func (u *SettingsUseCase) Get(ctx context.Context) (SettingsReading, error) {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return SettingsReading{}, err
	}
	return readSettings(snapshot), nil
}

// Save は .env に書くだけで、反映は次の起動に任せる。
//
// 操作にはしない。書き込みは一瞬で終わり、ワールドもコンテナも触らないので、
// 「同時にひとつだけ」の枠を消費させる理由がない（保持ポリシーの保存と同じ）。
//
// ただし他の操作の最中は断る。切替や作成も .env を書き換えるため、途中に
// 割り込むと相手の書き込みが競合で失敗し、止めたサーバーが戻らなくなる。
func (u *SettingsUseCase) Save(ctx context.Context, s settings.GameSettings) error {
	if active, busy := u.cfg.Operations.Active(); busy {
		return fmt.Errorf("%w（%s）", operations.ErrBusy, active.Kind)
	}
	return u.write(ctx, s)
}

// ApplyResult は SaveAndApply の結果。
type ApplyResult struct {
	Handle operations.Handle
	// Started は作り直しの操作を始めたか。停止中は書くだけで始めない。
	Started bool
}

// SaveAndApply は .env に書き、サーバーが動いていれば作り直して反映する。
//
// 止まっているサーバーは勝手に起動しない。書いておけば次の起動で反映される。
func (u *SettingsUseCase) SaveAndApply(ctx context.Context, s settings.GameSettings) (ApplyResult, error) {
	status, err := u.cfg.Runtime.Status(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	if !status.State.IsUp() {
		return ApplyResult{}, u.Save(ctx, s)
	}

	handle, err := u.cfg.Operations.Start(ctx, operation.KindServerApplySettings, applySteps(),
		func(ctx context.Context, r operations.Reporter) error {
			return u.runApply(ctx, r, s)
		})
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Handle: handle, Started: true}, nil
}

func applySteps() []string {
	return []string{
		"設定を書き込んでいます",
		"ワールドを保存しています",
		"サーバーを停止しています",
		"サーバーを起動しています",
		"起動を確認しています",
	}
}

// runApply は書き込んでから作り直す。
//
// 書き込みを先に行う。ワールドの切替は停止してから .env を書くが、設定の場合は
// 書き込みが競合で失敗したとき、まだ誰も切断されていない段階で止めたい。
// 停止の後で失敗すると、サーバーを止めたまま設定も変わっていない状態が残る。
func (u *SettingsUseCase) runApply(ctx context.Context, r operations.Reporter, s settings.GameSettings) error {
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.write(ctx, s); err != nil {
		return err
	}
	r.Logf(operation.LevelInfo, "難易度 %s、最大 %d 人、描画距離 %d、シミュレーション距離 %d",
		s.Difficulty(), s.MaxPlayers(), s.ViewDistance(), s.SimulationDistance())

	// 停止と起動は再起動と同じ手順を使う。.env の変更は作り直しでしか反映されない。
	if err := u.cfg.Lifecycle.down(ctx, r); err != nil {
		return err
	}
	return u.cfg.Lifecycle.up(ctx, r)
}

// write は 5 つのキーをまとめて書く。
//
// 読み込んでから書くまでの間に .env が手で編集されていれば、保存側が
// 競合として止める。上書きして人の編集を消すことはない。
func (u *SettingsUseCase) write(ctx context.Context, s settings.GameSettings) error {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return err
	}
	// ハードコアは書かない。画面から変えられない値で、決めるのはワールドの
	// 作成時だけ。ここで書き戻すと、読めなかった値を既定に倒した結果が
	// そのまま .env に焼き付いてしまう。
	next := snapshot.
		With(keyDifficulty, string(s.Difficulty())).
		With(keyMode, string(s.Mode())).
		With(keyMOTD, s.MOTD()).
		With(keyMaxPlayers, strconv.Itoa(s.MaxPlayers())).
		With(keyViewDistance, strconv.Itoa(s.ViewDistance())).
		With(keySimulationDistance, strconv.Itoa(s.SimulationDistance()))
	return u.cfg.Config.Save(ctx, next)
}

// readSettings は .env の値を読み、読めないものは既定値で補う。
func readSettings(snapshot port.ConfigSnapshot) SettingsReading {
	d := settings.Defaults()

	difficulty, w1 := readDifficulty(snapshot, d.Difficulty())
	mode, w5 := readMode(snapshot, d.Mode())
	motd := readString(snapshot, keyMOTD, d.MOTD())
	players, w2 := readInt(snapshot, keyMaxPlayers, d.MaxPlayers(), settings.MinMaxPlayers, settings.MaxMaxPlayers)
	view, w3 := readInt(snapshot, keyViewDistance, d.ViewDistance(), settings.MinDistance, settings.MaxDistance)
	sim, w4 := readInt(snapshot, keySimulationDistance, d.SimulationDistance(), settings.MinDistance, settings.MaxDistance)

	warnings := nonEmpty(w1, w5, w2, w3, w4)
	s, fixes := settle(settings.Params{
		Difficulty:         difficulty,
		Mode:               mode,
		MOTD:               motd,
		MaxPlayers:         players,
		ViewDistance:       view,
		SimulationDistance: sim,
		Hardcore:           readBool(snapshot, keyHardcore, d.Hardcore()),
	})
	return SettingsReading{Settings: s, Warnings: append(warnings, fixes...)}
}

// settle は個々の値は読めたが組み合わせが規則に合わない場合を直す。
//
// MOTD が規則外なら既定に戻し、シミュレーション距離が描画距離より遠ければ
// 描画距離に揃える。それでも作れなければ、全部を既定値にする。
func settle(p settings.Params) (settings.GameSettings, []string) {
	if s, err := settings.New(p); err == nil {
		return s, nil
	}

	var fixes []string
	d := settings.Defaults()

	// MOTD だけを既定に戻して通るなら、原因は MOTD にある。
	probe := p
	probe.SimulationDistance = min(p.SimulationDistance, p.ViewDistance)
	if _, err := settings.New(probe); err != nil {
		fixes = append(fixes, fmt.Sprintf("%s を読めなかったため、既定の %q を表示しています", keyMOTD, d.MOTD()))
		p.MOTD = d.MOTD()
	}
	if p.SimulationDistance > p.ViewDistance {
		fixes = append(fixes, fmt.Sprintf(
			"%s（%d）が %s（%d）より大きいため、%d として表示しています",
			keySimulationDistance, p.SimulationDistance, keyViewDistance, p.ViewDistance, p.ViewDistance))
		p.SimulationDistance = p.ViewDistance
	}

	if s, err := settings.New(p); err == nil {
		return s, fixes
	}
	return d, append(fixes, "設定を読めなかったため、既定値を表示しています")
}

// present は値が書かれているかを返す。空は「未設定」として compose が既定値で補う。
func present(snapshot port.ConfigSnapshot, key string) (string, bool) {
	raw, ok := snapshot.Get(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", false
	}
	return raw, true
}

func readDifficulty(snapshot port.ConfigSnapshot, fallback settings.Difficulty) (settings.Difficulty, string) {
	raw, ok := present(snapshot, keyDifficulty)
	if !ok {
		return fallback, ""
	}
	parsed, err := settings.ParseDifficulty(raw)
	if err != nil {
		return fallback, unreadable(keyDifficulty, raw, string(fallback))
	}
	return parsed, ""
}

func readMode(snapshot port.ConfigSnapshot, fallback settings.GameMode) (settings.GameMode, string) {
	raw, ok := present(snapshot, keyMode)
	if !ok {
		return fallback, ""
	}
	parsed, err := settings.ParseGameMode(raw)
	if err != nil {
		return fallback, unreadable(keyMode, raw, string(fallback))
	}
	return parsed, ""
}

// readBool は TRUE/FALSE を読む。
//
// compose が受け付けるのと同じく大文字小文字を問わない。読めない値は
// 既定に倒す。ハードコアは表示専用なので、警告までは出さない。
func readBool(snapshot port.ConfigSnapshot, key string, fallback bool) bool {
	raw, ok := present(snapshot, key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}

func readString(snapshot port.ConfigSnapshot, key, fallback string) string {
	if raw, ok := present(snapshot, key); ok {
		return raw
	}
	return fallback
}

func readInt(snapshot port.ConfigSnapshot, key string, fallback, low, high int) (int, string) {
	raw, ok := present(snapshot, key)
	if !ok {
		return fallback, ""
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < low || n > high {
		return fallback, unreadable(key, raw, strconv.Itoa(fallback))
	}
	return n, ""
}

func unreadable(key, raw, fallback string) string {
	return fmt.Sprintf("%s の値 %q を読めなかったため、既定の %s を表示しています", key, raw, fallback)
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
