package serverctl

import (
	"context"
	"errors"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// 待ち時間の上限。
//
// 停止は compose.yaml の stop_grace_period（60 秒）に余裕を足した値。
// 起動は README に記録された Done (11.800s) の実績に対して十分な余裕を取る。
const (
	stopTimeout  = 90 * time.Second
	startTimeout = 300 * time.Second
)

// LifecycleConfig は起動・停止のユースケースの依存。
type LifecycleConfig struct {
	Runtime    port.ContainerRuntime
	Console    port.ServerConsole
	Operations *operations.Manager
}

// LifecycleUseCase はサーバーの起動・停止・再起動。
type LifecycleUseCase struct {
	cfg LifecycleConfig
}

// NewLifecycleUseCase は LifecycleUseCase を作る。
func NewLifecycleUseCase(cfg LifecycleConfig) (*LifecycleUseCase, error) {
	if cfg.Runtime == nil || cfg.Operations == nil {
		return nil, errors.New("runtime と operations が必要です")
	}
	return &LifecycleUseCase{cfg: cfg}, nil
}

// Start はサーバーを起動する。
func (u *LifecycleUseCase) Start(ctx context.Context) (operations.Handle, error) {
	return u.cfg.Operations.Start(ctx, operation.KindServerStart,
		[]string{"サーバーを起動しています", "起動を確認しています"},
		func(ctx context.Context, r operations.Reporter) error {
			return u.up(ctx, r)
		})
}

// Stop はサーバーを停止する。
//
// 停止前にワールドを保存する。stop_grace_period が尊重されるので
// 保存なしでも壊れないはずだが、確実に書き出しておく。
func (u *LifecycleUseCase) Stop(ctx context.Context) (operations.Handle, error) {
	return u.cfg.Operations.Start(ctx, operation.KindServerStop,
		[]string{"ワールドを保存しています", "サーバーを停止しています"},
		func(ctx context.Context, r operations.Reporter) error {
			return u.down(ctx, r)
		})
}

// Restart はサーバーを停止してから起動する。
//
// docker compose restart は使わない。OVERRIDE_SERVER_PROPERTIES=TRUE の
// ため server.properties は起動時に .env から再生成されるが、
// restart では .env の変更が反映されない。
func (u *LifecycleUseCase) Restart(ctx context.Context) (operations.Handle, error) {
	return u.cfg.Operations.Start(ctx, operation.KindServerRestart,
		[]string{
			"ワールドを保存しています",
			"サーバーを停止しています",
			"サーバーを起動しています",
			"起動を確認しています",
		},
		func(ctx context.Context, r operations.Reporter) error {
			if err := u.down(ctx, r); err != nil {
				return err
			}
			return u.up(ctx, r)
		})
}

// up はコンテナを作り直して起動し、完了を待つ。
func (u *LifecycleUseCase) up(ctx context.Context, r operations.Reporter) error {
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.cfg.Runtime.Up(ctx, sinkOf(r)); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	return u.cfg.Runtime.WaitReady(ctx, startTimeout)
}

// down はワールドを保存してからコンテナを停止し、完了を待つ。
func (u *LifecycleUseCase) down(ctx context.Context, r operations.Reporter) error {
	if err := r.Step(); err != nil {
		return err
	}
	u.saveIfRunning(ctx, r)

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.cfg.Runtime.Down(ctx, sinkOf(r)); err != nil {
		return err
	}
	return u.cfg.Runtime.WaitStopped(ctx, stopTimeout)
}

// saveIfRunning はコンテナが動いていればワールドを保存する。
//
// 保存の失敗は停止を妨げない。停止できないより、直近の数秒が
// 失われる方がまだよい（stop_grace_period 中にサーバー自身も保存する）。
func (u *LifecycleUseCase) saveIfRunning(ctx context.Context, r operations.Reporter) {
	if u.cfg.Console == nil {
		return
	}
	status, err := u.cfg.Runtime.Status(ctx)
	if err != nil || !status.State.IsUp() {
		r.Logf(operation.LevelInfo, "サーバーが停止しているため保存を省略します")
		return
	}
	if err := u.cfg.Console.SaveAll(ctx); err != nil {
		r.Logf(operation.LevelWarn, "保存に失敗しましたが停止を続行します: %v", err)
	}
}

// sinkOf は docker の出力を進捗ログへ流す。
func sinkOf(r operations.Reporter) port.LogSink {
	return func(line string) { r.Logf(operation.LevelInfo, "%s", line) }
}
