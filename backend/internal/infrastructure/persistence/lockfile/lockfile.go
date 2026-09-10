// Package lockfile はプロセスをまたぐ排他とクラッシュの検出を行う。
//
// プロセス内の Mutex だけでは、systemd による再起動をまたげない。
// さらに「バックアップが中断された = save-off が残っているかもしれない」
// という事実も失われる。save-off を残したまま落ちると、以降の変更が
// ディスクに書かれないのに症状が何も出ないため、これは重大な取りこぼしになる。
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// ErrLocked は既に他の操作がロックを保持していることを表す。
var ErrLocked = errors.New("他の操作が実行中です")

// Meta はロックファイルに記録する内容。
type Meta struct {
	// OperationID は実行中の操作の識別子。
	OperationID string `json:"operation_id"`
	// Kind は操作の種類（表示用）。
	Kind string `json:"kind"`
	// PID はロックを取得したプロセスの ID。生存確認に使う。
	PID int `json:"pid"`
	// StartedAt は取得した時刻。
	StartedAt time.Time `json:"started_at"`
	// SaveDisabled は save-off を送った状態かどうか。
	//
	// これが真のまま残っていたら、次回起動時に save-on を送る必要がある。
	SaveDisabled bool `json:"save_disabled"`
}

// Handle は取得したロック。
type Handle struct {
	path     string
	released bool
}

// Acquire はロックを取得する。
//
// O_CREATE|O_EXCL による作成で排他する。既にファイルがあれば ErrLocked。
func Acquire(path string, m Meta) (*Handle, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w (%s)", ErrLocked, path)
		}
		return nil, fmt.Errorf("ロックを取得できません (%s): %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(m); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("ロックの内容を書けません: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("ロックを同期できません: %w", err)
	}

	return &Handle{path: path}, nil
}

// Release はロックを解放する。二重に呼んでも安全。
func (h *Handle) Release() error {
	if h.released {
		return nil
	}
	h.released = true

	if err := os.Remove(h.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ロックを解放できません (%s): %w", h.path, err)
	}
	return nil
}

// Path はロックファイルのパスを返す。
func (h *Handle) Path() string { return h.path }

// Inspect はロックファイルの内容を読む。
//
// 存在しない場合は found が偽になり、エラーにはしない。
// 解釈できない内容でもエラーにしない。手で編集されたり書き込み途中で
// 落ちたりしうるが、それで起動できなくなる方が困る。
// 解釈できなかった Meta は PID が 0 になり、IsStale が真を返す。
func Inspect(path string) (Meta, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, false, nil
		}
		return Meta{}, false, fmt.Errorf("ロックを読めません (%s): %w", path, err)
	}

	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		// 壊れている。存在はするので found は真、内容はゼロ値のまま返す。
		return Meta{}, true, nil
	}
	return m, true, nil
}

// IsStale は、ロックを取得したプロセスが既に存在しないかを返す。
//
// 真なら前回の実行が操作の途中で終了している。
func IsStale(m Meta) bool {
	if m.PID <= 0 {
		return true
	}
	return !processExists(m.PID)
}

// processExists はプロセスの生存を確認する。
//
// Unix では os.FindProcess が必ず成功するため、シグナル 0 を送って確かめる。
// 実際にシグナルは配送されず、存在確認だけが行われる。
func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// 他のユーザーのプロセスなら権限エラーになる。存在はしている。
	return errors.Is(err, os.ErrPermission)
}

// ForceRemove はロックファイルを削除する。
// 中断を検出したときに使う。存在しなくてもエラーにしない。
func ForceRemove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ロックを削除できません (%s): %w", path, err)
	}
	return nil
}

// Rewrite は既存のロックファイルの内容を書き換える。
//
// 操作の途中で save-off の状態が変わったときに使う。
// ロック自体は保持したまま内容だけを更新する。
func Rewrite(path string, m Meta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("ロックの内容を組み立てられません: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("ロックを書き換えられません (%s): %w", path, err)
	}
	return nil
}
