package operations

import (
	"fmt"
	"os"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// Reporter は操作の本体が進捗を報告するための窓口。
type Reporter interface {
	// Step は次のステップへ進める。
	Step() error
	// Logf は進捗ログを記録する。
	Logf(level operation.Level, format string, args ...any)
	// Bytes は処理済みと総バイト数を更新する。総量が不明なら total に 0。
	Bytes(done, total int64)
	// Attr は操作固有の情報を記録する。退避先のパスなど。
	Attr(key, value string)
	// MarkSaveDisabled は save-off を送った状態かどうかを記録する。
	//
	// ロックファイルに書くのは、プロセスが落ちた次の起動で
	// 「save-off が残っているかもしれない」と分かるようにするため。
	// save-off を残したまま落ちると、以降の変更がディスクに書かれないのに
	// 症状が何も出ない。
	MarkSaveDisabled(disabled bool)
}

/*
bytesInterval は進捗を記録する最短の間隔。

展開も作成もファイル 1 つごとに進捗を報告する。ワールドのファイル数が
そのままイベント数になるため、チャンクの多いワールドでは数千件になる。

イベントは 1 件ずつ完全なスナップショット（手順名の複製と属性の複製を
含む）を持つので、数千件はそのままメモリに残り、再接続のたびに全部を
送り直すことになる。さらに購読者のバッファを溢れさせ、最後のイベントが
落ちて画面が実行中のまま固まるところまで繋がる。

4 回/秒あれば進捗の帯は滑らかに見える。
*/
const bytesInterval = 250 * time.Millisecond

type reporter struct {
	manager *Manager
	op      *operation.Operation
	// lastBytesAt は最後に記録した進捗の時刻。manager.mu で守る。
	lastBytesAt time.Time
}

func (r *reporter) Step() error {
	r.manager.mu.Lock()
	err := r.op.Advance(r.manager.clock.Now())
	last := lastEvent(r.op)
	r.manager.mu.Unlock()

	if err != nil {
		return err
	}
	r.manager.publish(r.op.ID(), last)
	return nil
}

func (r *reporter) Logf(level operation.Level, format string, args ...any) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}

	r.manager.mu.Lock()
	r.op.Log(r.manager.clock.Now(), level, message)
	last := lastEvent(r.op)
	r.manager.mu.Unlock()

	r.manager.publish(r.op.ID(), last)
}

func (r *reporter) Bytes(done, total int64) {
	r.manager.mu.Lock()
	now := r.manager.clock.Now()
	if !r.shouldRecordBytesLocked(now, done, total) {
		r.manager.mu.Unlock()
		return
	}
	r.lastBytesAt = now
	r.op.SetBytes(now, done, total)
	last := lastEvent(r.op)
	r.manager.mu.Unlock()

	r.manager.publish(r.op.ID(), last)
}

// shouldRecordBytesLocked は進捗を記録するかを決める。mu を保持して呼ぶ。
//
// 最初と最後は必ず記録する。最後を落とすと、進捗の帯が途中で止まった
// まま操作だけが完了することになる。
func (r *reporter) shouldRecordBytesLocked(now time.Time, done, total int64) bool {
	switch {
	case r.lastBytesAt.IsZero():
		return true
	case total > 0 && done >= total:
		return true
	default:
		return now.Sub(r.lastBytesAt) >= bytesInterval
	}
}

func (r *reporter) Attr(key, value string) {
	r.manager.mu.Lock()
	defer r.manager.mu.Unlock()

	r.op.SetAttribute(key, value)
}

func (r *reporter) MarkSaveDisabled(disabled bool) {
	r.manager.mu.Lock()
	if r.manager.lock == nil {
		r.manager.mu.Unlock()
		return
	}
	r.manager.lockMeta.SaveDisabled = disabled
	meta := r.manager.lockMeta
	lock := r.manager.cfg.Lock
	r.manager.mu.Unlock()

	// 失敗しても操作は続ける。ここで止めると、記録できないという理由で
	// バックアップそのものが失敗することになる。
	_ = lock.Update(meta)
}

func lastEvent(op *operation.Operation) operation.Event {
	events := op.Events()
	if len(events) == 0 {
		return operation.Event{}
	}
	return events[len(events)-1]
}

func processID() int { return os.Getpid() }

var _ Reporter = (*reporter)(nil)
