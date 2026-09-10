package compose

import (
	"context"
	"strings"
	"time"
)

// doneMarker は Paper が起動を終えたときにログへ出す文字列。
// 例: Done (7.575s)! For help, type "help"
const doneMarker = "Done ("

// logTailLines は起動判定で確認するログの行数。
const logTailLines = "50"

// WaitReady はサーバーの起動完了を待つ。
//
// 判定はログの "Done (" と、イメージ同梱のヘルスチェックの合格の**いずれか**。
// ログだけに頼るとローテーションやバッファリングで取りこぼしうるし、
// ヘルスチェックだけに頼ると合格までの猶予が読めない。両方を見る。
func (r *Runner) WaitReady(ctx context.Context, timeout time.Duration) error {
	return r.poll(ctx, timeout, "起動", func() (bool, error) {
		if healthy, err := r.isHealthy(ctx); err != nil {
			return false, err
		} else if healthy {
			return true, nil
		}
		return r.logsContainDone(ctx), nil
	})
}

func (r *Runner) isHealthy(ctx context.Context) (bool, error) {
	st, err := r.Status(ctx)
	if err != nil {
		return false, err
	}
	return st.Healthy, nil
}

// logsContainDone は直近のログに起動完了の印があるかを返す。
//
// ログの取得に失敗しても、それ自体は起動判定の失敗にしない。
// ヘルスチェック側で判定できる可能性が残っている。
func (r *Runner) logsContainDone(ctx context.Context) bool {
	args := append(r.baseArgs(), "logs", "--tail", logTailLines, r.cfg.Service)

	out, err := r.runCapture(ctx, args...)
	if err != nil {
		return false
	}
	return strings.Contains(out, doneMarker)
}
