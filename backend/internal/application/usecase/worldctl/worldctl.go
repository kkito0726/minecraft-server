// Package worldctl はワールドの一覧・切替・作成・複製・改名・削除。
//
// ワールドの実体は data/<名前>/ で、稼働させるものは .env の MC_LEVEL で
// 決まる。切り替えてもディレクトリは移動しないため、切替前のワールドは
// そのまま残る（2026-09-09 に実機で確認済み）。
package worldctl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// 設定のキー。
const (
	keyLevel = "MC_LEVEL"
	keySeed  = "MC_SEED"
)

// defaultLevel は MC_LEVEL が未設定のときのワールド名。
// compose.yaml の ${MC_LEVEL:-world} と合わせる。
const defaultLevel = "world"

// 待ち時間の上限。
//
// 新規ワールドの生成は Pi では相当に時間がかかるため、既存のワールドを
// 開くときより長く待つ。
const (
	stopTimeout    = 90 * time.Second
	startTimeout   = 300 * time.Second
	generateTimout = 900 * time.Second
)

// Config は UseCase の依存。
type Config struct {
	Runtime    port.ContainerRuntime
	Console    port.ServerConsole
	Worlds     port.WorldRepository
	Config     port.ServerConfig
	Levels     port.LevelReader
	Operations *operations.Manager
}

// UseCase はワールドの操作。
type UseCase struct {
	cfg Config
}

// New は UseCase を作る。
func New(cfg Config) (*UseCase, error) {
	if cfg.Runtime == nil || cfg.Worlds == nil || cfg.Config == nil || cfg.Operations == nil {
		return nil, errors.New("runtime / worlds / config / operations が必要です")
	}
	return &UseCase{cfg: cfg}, nil
}

// Switch は稼働させるワールドを切り替える。
//
// 手順は「保存 → 停止 → .env 書き換え → 起動 → 確認」。
// .env を先に書き換えると、停止に失敗したときに設定だけが変わった
// 状態になる。停止を済ませてから設定を触る。
//
// 切替前のワールドのディレクトリは移動も削除もしない。
func (u *UseCase) Switch(ctx context.Context, name string) (operations.Handle, error) {
	target, err := world.NewName(name)
	if err != nil {
		return operations.Handle{}, err
	}

	current, err := u.currentLevel(ctx)
	if err != nil {
		return operations.Handle{}, err
	}

	// 同じワールドなら何もしない。停止と起動を挟む理由がない。
	if current == target {
		return u.noop(ctx, operation.KindWorldSwitch,
			fmt.Sprintf("既に %s で稼働しています", target))
	}

	exists, err := u.cfg.Worlds.Exists(ctx, target)
	if err != nil {
		return operations.Handle{}, err
	}

	return u.cfg.Operations.Start(ctx, operation.KindWorldSwitch, switchSteps(),
		func(ctx context.Context, r operations.Reporter) error {
			return u.runSwitch(ctx, r, target, exists)
		})
}

func switchSteps() []string {
	return []string{
		"ワールドを保存しています",
		"サーバーを停止しています",
		"設定を書き換えています",
		"サーバーを起動しています",
		"起動を確認しています",
	}
}

func (u *UseCase) runSwitch(
	ctx context.Context,
	r operations.Reporter,
	target world.Name,
	exists bool,
) error {
	if !exists {
		r.Logf(operation.LevelWarn,
			"%s は存在しないため、サーバーが新しく生成します", target)
	}

	if err := r.Step(); err != nil {
		return err
	}
	u.saveIfRunning(ctx, r)

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.stop(ctx, r); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.setLevel(ctx, target); err != nil {
		return err
	}
	r.Attr("world_name", target.String())

	return u.startAndWait(ctx, r, exists)
}

// startAndWait は起動して完了を待つ。
// 新規生成の場合は待ち時間を長く取る。
func (u *UseCase) startAndWait(ctx context.Context, r operations.Reporter, exists bool) error {
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.cfg.Runtime.Up(ctx, sinkOf(r)); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	timeout := startTimeout
	if !exists {
		timeout = generateTimout
		r.Logf(operation.LevelInfo, "新しいワールドの生成には時間がかかります")
	}
	return u.cfg.Runtime.WaitReady(ctx, timeout)
}

func (u *UseCase) stop(ctx context.Context, r operations.Reporter) error {
	if err := u.cfg.Runtime.Down(ctx, sinkOf(r)); err != nil {
		return err
	}
	return u.cfg.Runtime.WaitStopped(ctx, stopTimeout)
}

// saveIfRunning はコンテナが動いていればワールドを保存する。
// 保存の失敗は停止を妨げない。stop_grace_period 中にサーバー自身も保存する。
func (u *UseCase) saveIfRunning(ctx context.Context, r operations.Reporter) {
	if u.cfg.Console == nil {
		return
	}
	status, err := u.cfg.Runtime.Status(ctx)
	if err != nil || !status.State.IsUp() {
		r.Logf(operation.LevelInfo, "サーバーが停止しているため保存を省略します")
		return
	}
	if err := u.cfg.Console.SaveAll(ctx); err != nil {
		r.Logf(operation.LevelWarn, "保存に失敗しましたが続行します: %v", err)
	}
}

// setLevel は .env の MC_LEVEL を書き換える。
//
// あわせて MC_SEED を空に戻す。新規ワールドの生成待ちは最大 900 秒に及び、
// その間にプロセスが停止するとシードが残る。残ったまま次のワールドを
// 作ると、意図せず同じ地形になる。
func (u *UseCase) setLevel(ctx context.Context, name world.Name) error {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return err
	}
	updated := snapshot.With(keyLevel, name.String()).With(keySeed, "")
	return u.cfg.Config.Save(ctx, updated)
}

// currentLevel は .env の MC_LEVEL を読む。
func (u *UseCase) currentLevel(ctx context.Context) (world.Name, error) {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return world.Name{}, err
	}

	value, ok := snapshot.Get(keyLevel)
	if !ok || value == "" {
		value = defaultLevel
	}
	return world.NewName(value)
}

// noop は何もせずに成功する操作を作る。
// 画面には「実行して完了した」ように見せ、利用者を混乱させない。
func (u *UseCase) noop(
	ctx context.Context,
	kind operation.Kind,
	message string,
) (operations.Handle, error) {
	return u.cfg.Operations.Start(ctx, kind, []string{message},
		func(_ context.Context, r operations.Reporter) error {
			return r.Step()
		})
}

// sinkOf は docker の出力を進捗ログへ流す。
func sinkOf(r operations.Reporter) port.LogSink {
	return func(line string) { r.Logf(operation.LevelInfo, "%s", line) }
}
