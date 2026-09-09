package dotenv_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 書き込めないディレクトリでは、元の .env を壊さずに失敗する。
func TestStoreSaveIntoReadOnlyDir(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("パーミッションによる書き込み拒否を再現できない環境")
	}

	store, path := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := store.Save(context.Background(), snap.WithValue("MC_LEVEL", "x")); err == nil {
		t.Fatal("エラーになるはず")
	}

	// 元の内容が無事であること
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "MC_LEVEL=world") {
		t.Error("失敗したのに元の内容が壊れている")
	}
}

// 削除されたファイルへの Save は、衝突ではなく明確なエラーになる。
func TestStoreSaveAfterFileRemoved(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(context.Background(), snap.WithValue("MC_LEVEL", "x")); err == nil {
		t.Error("エラーになるはず")
	}
}

// Load でキャンセル済みの context を渡したら読まない。
func TestStoreLoadRespectsContext(t *testing.T) {
	t.Parallel()

	store, _ := newStore(t, 0o600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Load(ctx); err == nil {
		t.Error("エラーになるはず")
	}
}

// WithoutValue はキーを取り除いた新しい Snapshot を返す。元は変わらない。
func TestSnapshotWithoutValue(t *testing.T) {
	t.Parallel()

	store, _ := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	after := snap.WithoutValue("MC_SEED")

	if _, ok := after.File().Get("MC_SEED"); ok {
		t.Error("MC_SEED が残っている")
	}
	if _, ok := snap.File().Get("MC_SEED"); !ok {
		t.Error("元の Snapshot が変更された")
	}
	// 楽観ロックの判定材料が引き継がれていること
	if !after.LoadedAt().Equal(snap.LoadedAt()) {
		t.Error("LoadedAt が引き継がれていない")
	}
}

// Path は対象のパスを返す。ログやエラーメッセージで使う。
func TestStorePath(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	if got := store.Path(); got != path {
		t.Errorf("Path() が %q。%q のはず", got, path)
	}
}

// 読めないファイル（パーミッションなし）は明確なエラーになる。
func TestStoreLoadUnreadableFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("パーミッションによる読み取り拒否を再現できない環境")
	}

	store, path := newStore(t, 0o000)
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	if _, err := store.Load(context.Background()); err == nil {
		t.Error("エラーになるはず")
	}
}
