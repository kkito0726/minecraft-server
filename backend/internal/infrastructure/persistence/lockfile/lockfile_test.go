package lockfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".admin-console.lock")
}

func meta() lockfile.Meta {
	return lockfile.Meta{
		OperationID:  "op-1",
		Kind:         "バックアップの取得",
		PID:          os.Getpid(),
		StartedAt:    time.Now(),
		SaveDisabled: true,
	}
}

func TestAcquireAndRelease(t *testing.T) {
	t.Parallel()

	path := lockPath(t)

	h, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatalf("Acquire に失敗: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("ロックファイルが作られていない: %v", err)
	}

	if err := h.Release(); err != nil {
		t.Fatalf("Release に失敗: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Release 後もロックファイルが残っている")
	}
}

// 二重取得を防ぐ。バックアップと復元が同時に走ると data/ が壊れる。
func TestAcquireTwiceFails(t *testing.T) {
	t.Parallel()

	path := lockPath(t)

	h, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	if _, err := lockfile.Acquire(path, meta()); !errors.Is(err, lockfile.ErrLocked) {
		t.Errorf("ErrLocked を期待したが %v", err)
	}
}

// Release 後は再取得できる。
func TestAcquireAfterRelease(t *testing.T) {
	t.Parallel()

	path := lockPath(t)

	h, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Release(); err != nil {
		t.Fatal(err)
	}

	h2, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatalf("再取得できるはずが %v", err)
	}
	_ = h2.Release()
}

// 記録した内容を読み戻せる。中断の検出に使う。
func TestInspect(t *testing.T) {
	t.Parallel()

	path := lockPath(t)
	m := meta()

	h, err := lockfile.Acquire(path, m)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	got, found, err := lockfile.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect に失敗: %v", err)
	}
	if !found {
		t.Fatal("ロックが見つからない")
	}
	if got.OperationID != m.OperationID || got.Kind != m.Kind || got.PID != m.PID {
		t.Errorf("内容が %+v", got)
	}
	if !got.SaveDisabled {
		t.Error("SaveDisabled が失われている")
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt が失われている")
	}
}

func TestInspectMissing(t *testing.T) {
	t.Parallel()

	_, found, err := lockfile.Inspect(lockPath(t))
	if err != nil {
		t.Fatalf("存在しないだけならエラーにしないはず: %v", err)
	}
	if found {
		t.Error("見つかってはいけない")
	}
}

// バックアップの途中でプロセスが落ちると、save-off を残したまま
// ロックファイルだけが残る。次回起動時にこれを検出して警告する。
func TestIsStale(t *testing.T) {
	t.Parallel()

	t.Run("生きているプロセスは stale でない", func(t *testing.T) {
		t.Parallel()
		m := meta()
		m.PID = os.Getpid()
		if lockfile.IsStale(m) {
			t.Error("自分自身が stale と判定された")
		}
	})

	t.Run("存在しないプロセスは stale", func(t *testing.T) {
		t.Parallel()
		m := meta()
		// 使われていない可能性が非常に高い PID
		m.PID = 999999
		if !lockfile.IsStale(m) {
			t.Error("存在しないプロセスが stale と判定されない")
		}
	})

	t.Run("PID が不正なら stale", func(t *testing.T) {
		t.Parallel()
		m := meta()
		m.PID = 0
		if !lockfile.IsStale(m) {
			t.Error("PID 0 が stale と判定されない")
		}
	})
}

// 壊れたロックファイルは stale として扱い、起動を妨げない。
// 手で編集されたり書き込み途中で落ちたりしうる。
func TestInspectCorrupted(t *testing.T) {
	t.Parallel()

	path := lockPath(t)
	if err := os.WriteFile(path, []byte("これは JSON ではない"), 0o600); err != nil {
		t.Fatal(err)
	}

	m, found, err := lockfile.Inspect(path)
	if err != nil {
		t.Fatalf("壊れていてもエラーにしないはず: %v", err)
	}
	if !found {
		t.Error("ファイルは存在するので found は真のはず")
	}
	if !lockfile.IsStale(m) {
		t.Error("解釈できないロックは stale として扱うはず")
	}
}

// 強制的な削除。中断を検出したときに使う。
func TestForceRemove(t *testing.T) {
	t.Parallel()

	path := lockPath(t)
	h, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatal(err)
	}
	_ = h // Release せずに残す

	if err := lockfile.ForceRemove(path); err != nil {
		t.Fatalf("ForceRemove に失敗: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("削除されていない")
	}

	// 存在しないものを消しても失敗しない
	if err := lockfile.ForceRemove(path); err != nil {
		t.Errorf("存在しない場合もエラーにしないはず: %v", err)
	}
}

// Release を二重に呼んでも壊れない。defer と明示的な呼び出しが重なりうる。
func TestReleaseTwice(t *testing.T) {
	t.Parallel()

	path := lockPath(t)
	h, err := lockfile.Acquire(path, meta())
	if err != nil {
		t.Fatal(err)
	}

	if err := h.Release(); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(); err != nil {
		t.Errorf("2 回目の Release でエラー: %v", err)
	}
}
