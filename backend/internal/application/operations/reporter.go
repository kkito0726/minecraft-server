package operations

import (
	"fmt"
	"os"

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

type reporter struct {
	manager *Manager
	op      *operation.Operation
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
	r.op.SetBytes(r.manager.clock.Now(), done, total)
	last := lastEvent(r.op)
	r.manager.mu.Unlock()

	r.manager.publish(r.op.ID(), last)
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
