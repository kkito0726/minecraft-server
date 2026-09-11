// Package reconcile は save-on の取りこぼしを回復する。
//
// ここが守るのは、この設計で最も静かに壊れる不変条件。
//
// バックアップは save-off でワールドの保存を止めてから取る。途中で
// プロセスが落ちると save-off が残るが、**症状が何も出ない**。
// サーバーは動き続け、プレイヤーは遊べ、ログにも何も出ない。
// ただ、それ以降の変更が一切ディスクに書かれない。
// 次の再起動で数時間分のプレイが消えて初めて気づくことになる。
//
// RCON には「保存が有効か」を問い合わせる手段がない。そのため状態を
// 照会するのではなく、冪等な save-on を無条件に送る。
//   - バックエンドの起動時に 1 回
//   - コンテナが非 healthy から healthy へ遷移するたびに 1 回
//
// 前者だけでは、バックエンドが MC より先に起動したときに送信が失敗して
// 終わる。後者がその補償になっている。
package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// defaultInterval はヘルスチェックを確認する間隔。
const defaultInterval = 10 * time.Second

// Console は保存の再開を送る先。
type Console interface {
	SaveOn(ctx context.Context) error
}

// Health はコンテナのヘルスチェックの状態を返す。
type Health interface {
	Healthy(ctx context.Context) (bool, error)
}

// Config は Reconciler の設定。
type Config struct {
	// Console は save-on の送信先。
	Console Console
	// Health はヘルスチェックの状態の取得元。
	Health Health
	// Lock はプロセスをまたぐ排他。中断の検出に使う。
	Lock port.OperationLock
	// Interval はヘルスチェックを確認する間隔。0 なら既定値。
	Interval time.Duration
	// Logger は記録先。nil なら既定のロガー。
	Logger *slog.Logger
}

// Result は 1 回の実行結果。
type Result struct {
	// InterruptedDetected は前回の実行が操作の途中で終了していたか。
	//
	// 真なら画面に警告を出す。save-off が残っていた可能性があり、
	// 直前のバックアップが不完全かもしれないため。
	InterruptedDetected bool
	// InterruptedKind は中断された操作の種類（表示用）。
	InterruptedKind string
	// SaveWasDisabled は中断時に save-off の状態だったか。
	SaveWasDisabled bool
}

// Reconciler は save-on の再送と中断の検出を行う。
type Reconciler struct {
	cfg      Config
	logger   *slog.Logger
	interval time.Duration
}

// New は Reconciler を作る。
func New(cfg Config) (*Reconciler, error) {
	if cfg.Console == nil {
		return nil, errors.New("Console が指定されていません")
	}
	if cfg.Lock == nil {
		return nil, errors.New("ロックが指定されていません")
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Reconciler{cfg: cfg, logger: logger, interval: interval}, nil
}

// RunOnce は起動時の回復処理を行う。
//
// save-on の送信に失敗しても異常にしない。バックエンドは MC より先に
// 起動しうるし、その場合は Loop が healthy への遷移で送り直す。
func (r *Reconciler) RunOnce(ctx context.Context) (Result, error) {
	result := r.detectInterruption()
	r.sendSaveOn(ctx, "起動時")
	return result, nil
}

// detectInterruption は前回の実行が途中で終了していないかを調べる。
func (r *Reconciler) detectInterruption() Result {
	meta, found, err := r.cfg.Lock.Inspect()
	if err != nil {
		r.logger.Warn("ロックを読めませんでした", "error", err)
		return Result{}
	}
	if !found {
		return Result{}
	}
	if !r.cfg.Lock.IsStale(meta) {
		// 別のプロセスが実行中。多重起動の可能性があるが、
		// ここで消すと動いている操作のロックを奪うことになる。
		r.logger.Warn("実行中のロックがあります",
			"operation_id", meta.OperationID, "pid", meta.PID)
		return Result{}
	}

	r.logger.Warn("中断された操作を検出しました",
		"operation_id", meta.OperationID,
		"kind", meta.Kind,
		"save_disabled", meta.SaveDisabled,
		"started_at", meta.StartedAt)

	if err := r.cfg.Lock.ForceRemove(); err != nil {
		r.logger.Error("stale なロックを削除できませんでした", "error", err)
	}

	return Result{
		InterruptedDetected: true,
		InterruptedKind:     meta.Kind,
		SaveWasDisabled:     meta.SaveDisabled,
	}
}

// sendSaveOn は save-on を送る。失敗しても記録するだけで続行する。
func (r *Reconciler) sendSaveOn(ctx context.Context, reason string) {
	if err := r.cfg.Console.SaveOn(ctx); err != nil {
		// コンテナが停止していれば当然失敗する。異常ではない。
		r.logger.Debug("save-on を送れませんでした（サーバー未起動の可能性）",
			"reason", reason, "error", err)
		return
	}
	r.logger.Info("save-on を送信しました", "reason", reason)
}

/*
savingIsIntentionallyOff は、実行中の操作が意図して保存を止めているかを返す。

hot バックアップは save-off のまま zip を固める。その間コンテナは
healthy のままなので通常は遷移が起きないが、I/O 負荷でヘルスチェックが
一度落ちて戻ると「非 healthy → healthy」になる。Pi では起こりうる。

そこで save-on を送ると、**zip を書いている最中に保存が再開される**。
書き込み途中の region ファイルが取り込まれ、静かに壊れたバックアップが
できあがる。save-off がそもそも防いでいたはずのものになる。

判断できないとき（ロックを読めない、ロックが無い）は送る側に倒す。
送りすぎは冪等なので無害だが、送らなすぎは save-off の残留につながる。
*/
func (r *Reconciler) savingIsIntentionallyOff() bool {
	if r.cfg.Lock == nil {
		return false
	}
	meta, found, err := r.cfg.Lock.Inspect()
	if err != nil || !found {
		return false
	}
	// 死んだプロセスのロックは中断の跡。むしろ送らないといけない。
	if r.cfg.Lock.IsStale(meta) {
		return false
	}
	return meta.SaveDisabled
}

// Loop は healthy への遷移を監視して save-on を送り直す。
//
// ctx がキャンセルされるまで動き続ける。
func (r *Reconciler) Loop(ctx context.Context) {
	if r.cfg.Health == nil {
		return
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	wasHealthy := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		healthy, err := r.cfg.Health.Healthy(ctx)
		if err != nil {
			r.logger.Debug("ヘルスチェックを取得できませんでした", "error", err)
			continue
		}

		// 非 healthy から healthy への遷移でだけ送る。
		// 合格が続く間に送り続けると、無駄な RCON 呼び出しが増える。
		if healthy && !wasHealthy && !r.savingIsIntentionallyOff() {
			r.sendSaveOn(ctx, "healthy への遷移")
		}
		wasHealthy = healthy
	}
}
