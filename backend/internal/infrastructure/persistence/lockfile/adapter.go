package lockfile

import (
	stderrors "errors"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// Lock は port.OperationLock の実装。
//
// application 層は具体的なファイル操作を知らず、このインターフェース越しに
// 排他を扱う。テストではメモリ上の実装に差し替えられる。
type Lock struct {
	path string
}

// NewLock は指定したパスを使う Lock を作る。
func NewLock(path string) *Lock { return &Lock{path: path} }

// Acquire はロックを取得する。
func (l *Lock) Acquire(meta port.LockMeta) (port.LockHandle, error) {
	h, err := Acquire(l.path, toMeta(meta))
	if err != nil {
		if isLocked(err) {
			return nil, port.ErrLocked
		}
		return nil, err
	}
	return h, nil
}

// Update は保持中のロックの内容を書き換える。
func (l *Lock) Update(meta port.LockMeta) error { return Rewrite(l.path, toMeta(meta)) }

// Inspect はロックの内容を読む。
func (l *Lock) Inspect() (port.LockMeta, bool, error) {
	m, found, err := Inspect(l.path)
	if err != nil || !found {
		return port.LockMeta{}, found, err
	}
	return fromMeta(m), true, nil
}

// IsStale はロックを取得したプロセスが既に存在しないかを返す。
func (l *Lock) IsStale(meta port.LockMeta) bool { return IsStale(toMeta(meta)) }

// ForceRemove はロックを削除する。
func (l *Lock) ForceRemove() error { return ForceRemove(l.path) }

// Path はロックファイルのパスを返す。
func (l *Lock) Path() string { return l.path }

func toMeta(m port.LockMeta) Meta {
	return Meta{
		OperationID:  m.OperationID,
		Kind:         m.Kind,
		PID:          m.PID,
		StartedAt:    m.StartedAt,
		SaveDisabled: m.SaveDisabled,
	}
}

func fromMeta(m Meta) port.LockMeta {
	return port.LockMeta{
		OperationID:  m.OperationID,
		Kind:         m.Kind,
		PID:          m.PID,
		StartedAt:    m.StartedAt,
		SaveDisabled: m.SaveDisabled,
	}
}

func isLocked(err error) bool {
	return err != nil && errorsIs(err, ErrLocked)
}

var _ port.OperationLock = (*Lock)(nil)

// errorsIs は errors.Is の別名。import の並びを adapter.go に閉じるために置く。
func errorsIs(err, target error) bool { return stderrors.Is(err, target) }
