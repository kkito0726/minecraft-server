package lockfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

func portMeta() port.LockMeta {
	return port.LockMeta{
		OperationID:  "op-1",
		Kind:         "バックアップの取得",
		PID:          os.Getpid(),
		StartedAt:    time.Now(),
		SaveDisabled: true,
	}
}

// アダプタは port.OperationLock として振る舞う。
// ユースケースはこのインターフェース越しにしか排他を扱わない。
func TestLockAdapterRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".lock")
	l := lockfile.NewLock(path)

	if l.Path() != path {
		t.Errorf("Path() が %q", l.Path())
	}

	handle, err := l.Acquire(portMeta())
	if err != nil {
		t.Fatalf("Acquire に失敗: %v", err)
	}

	got, found, err := l.Inspect()
	if err != nil || !found {
		t.Fatalf("Inspect に失敗: err=%v found=%v", err, found)
	}
	if got.OperationID != "op-1" || got.Kind != "バックアップの取得" {
		t.Errorf("内容が %+v", got)
	}
	if !got.SaveDisabled {
		t.Error("SaveDisabled が失われている")
	}
	if l.IsStale(got) {
		t.Error("生きているプロセスが stale と判定された")
	}

	if err := handle.Release(); err != nil {
		t.Fatalf("Release に失敗: %v", err)
	}
	if _, found, _ := l.Inspect(); found {
		t.Error("解放後もロックが残っている")
	}
}

// 二重取得は port.ErrLocked になる。
// ユースケースは lockfile 固有のエラーを知らない。
func TestLockAdapterReturnsPortError(t *testing.T) {
	t.Parallel()

	l := lockfile.NewLock(filepath.Join(t.TempDir(), ".lock"))

	handle, err := l.Acquire(portMeta())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Release() }()

	if _, err := l.Acquire(portMeta()); !errors.Is(err, port.ErrLocked) {
		t.Errorf("port.ErrLocked を期待したが %v", err)
	}
}

// Update は保持中のロックの内容だけを差し替える。
func TestLockAdapterUpdate(t *testing.T) {
	t.Parallel()

	l := lockfile.NewLock(filepath.Join(t.TempDir(), ".lock"))
	meta := portMeta()
	meta.SaveDisabled = false

	handle, err := l.Acquire(meta)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Release() }()

	meta.SaveDisabled = true
	if err := l.Update(meta); err != nil {
		t.Fatalf("Update に失敗: %v", err)
	}

	got, _, err := l.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if !got.SaveDisabled {
		t.Error("SaveDisabled が反映されていない")
	}
	// ロックは保持されたまま
	if _, err := l.Acquire(meta); !errors.Is(err, port.ErrLocked) {
		t.Error("Update でロックが解放されている")
	}
}

func TestLockAdapterForceRemove(t *testing.T) {
	t.Parallel()

	l := lockfile.NewLock(filepath.Join(t.TempDir(), ".lock"))
	if _, err := l.Acquire(portMeta()); err != nil {
		t.Fatal(err)
	}

	if err := l.ForceRemove(); err != nil {
		t.Fatalf("ForceRemove に失敗: %v", err)
	}
	if _, found, _ := l.Inspect(); found {
		t.Error("削除されていない")
	}
}

// 存在しないプロセスのロックは stale。
func TestLockAdapterIsStale(t *testing.T) {
	t.Parallel()

	l := lockfile.NewLock(filepath.Join(t.TempDir(), ".lock"))
	meta := portMeta()
	meta.PID = 999999

	if !l.IsStale(meta) {
		t.Error("存在しないプロセスが stale と判定されない")
	}
}

func TestLockAdapterInspectMissing(t *testing.T) {
	t.Parallel()

	l := lockfile.NewLock(filepath.Join(t.TempDir(), ".lock"))
	_, found, err := l.Inspect()
	if err != nil {
		t.Errorf("存在しないだけならエラーにしないはず: %v", err)
	}
	if found {
		t.Error("見つかってはいけない")
	}
}
