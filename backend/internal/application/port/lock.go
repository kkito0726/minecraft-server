package port

import (
	"errors"
	"time"
)

// ErrLocked は既に他の操作がロックを保持していることを表す。
var ErrLocked = errors.New("他の操作が実行中です")

// LockMeta はロックに記録する内容。
//
// プロセスをまたいで中断を検出するために永続化される。
// とくに SaveDisabled が重要で、これが残っていると
// 「save-off を送ったまま落ちた」ことが分かる。
type LockMeta struct {
	// OperationID は実行中の操作の識別子。
	OperationID string
	// Kind は操作の種類（表示用）。
	Kind string
	// PID はロックを取得したプロセスの ID。生存確認に使う。
	PID int
	// StartedAt は取得した時刻。
	StartedAt time.Time
	// SaveDisabled は save-off を送った状態かどうか。
	SaveDisabled bool
}

// LockHandle は取得したロック。
type LockHandle interface {
	// Release はロックを解放する。二重に呼んでも安全。
	Release() error
}

// OperationLock はプロセスをまたぐ排他。
//
// プロセス内の Mutex だけでは systemd による再起動をまたげず、
// 「操作が中断された」という事実も失われる。
type OperationLock interface {
	// Acquire はロックを取得する。取得できなければ ErrLocked を返す。
	Acquire(meta LockMeta) (LockHandle, error)
	// Update は保持中のロックの内容を書き換える。
	// 操作の途中で save-off の状態が変わったときに使う。
	Update(meta LockMeta) error
	// Inspect はロックの内容を読む。存在しなければ found が偽。
	Inspect() (meta LockMeta, found bool, err error)
	// IsStale はロックを取得したプロセスが既に存在しないかを返す。
	IsStale(meta LockMeta) bool
	// ForceRemove はロックを削除する。中断を検出したときに使う。
	ForceRemove() error
}
