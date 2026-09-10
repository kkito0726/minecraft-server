package lockfile_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

// 書き込めない場所ではロックを取得できない。
func TestAcquireInUnwritableDir(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("パーミッションによる書き込み拒否を再現できない環境")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := lockfile.Acquire(filepath.Join(dir, ".lock"), lockfile.Meta{PID: os.Getpid()}); err == nil {
		t.Error("エラーになるはず")
	}
}

// 読めないファイルは明確なエラーにする。
func TestInspectUnreadable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("パーミッションによる読み取り拒否を再現できない環境")
	}

	path := filepath.Join(t.TempDir(), ".lock")
	if err := os.WriteFile(path, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	if _, _, err := lockfile.Inspect(path); err == nil {
		t.Error("エラーになるはず")
	}
}

// Rewrite は内容だけを差し替える。ロックは保持したまま。
func TestRewrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".lock")
	m := lockfile.Meta{OperationID: "op-1", PID: os.Getpid()}

	h, err := lockfile.Acquire(path, m)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	m.SaveDisabled = true
	if err := lockfile.Rewrite(path, m); err != nil {
		t.Fatalf("Rewrite に失敗: %v", err)
	}

	got, found, err := lockfile.Inspect(path)
	if err != nil || !found {
		t.Fatalf("読み戻せない: err=%v found=%v", err, found)
	}
	if !got.SaveDisabled {
		t.Error("SaveDisabled が反映されていない")
	}
	if got.OperationID != "op-1" {
		t.Errorf("OperationID が %q", got.OperationID)
	}

	// ロックは保持されたまま（再取得できない）
	if _, err := lockfile.Acquire(path, m); err == nil {
		t.Error("Rewrite でロックが解放されている")
	}
}

func TestHandlePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".lock")
	h, err := lockfile.Acquire(path, lockfile.Meta{PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	if h.Path() != path {
		t.Errorf("Path() が %q", h.Path())
	}
}
