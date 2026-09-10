package dotenv_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/config/dotenv"
)

// newStore はフィクスチャを置いた一時ディレクトリと Store を用意する。
func newStore(t *testing.T, perm os.FileMode) (*dotenv.FileStore, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, loadFixture(t), perm); err != nil {
		t.Fatal(err)
	}
	return dotenv.NewFileStore(path), path
}

func TestStoreLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load に失敗: %v", err)
	}
	if err := store.Save(context.Background(), snap.WithValue("MC_LEVEL", "creative")); err != nil {
		t.Fatalf("Save に失敗: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	added, removed := diffLines(string(original), string(after))
	if len(added) != 1 || len(removed) != 1 {
		t.Errorf("変更は 1 行のみのはず。追加 %q / 削除 %q", added, removed)
	}
}

// .env は RCON_PASSWORD を持つ。一時ファイル経由で書き戻すときに
// パーミッションが緩むと、他のユーザーから読めるようになる。
func TestStoreKeepsFileMode(t *testing.T) {
	t.Parallel()

	for _, perm := range []os.FileMode{0o600, 0o640} {
		t.Run(perm.String(), func(t *testing.T) {
			t.Parallel()
			store, path := newStore(t, perm)

			snap, err := store.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(context.Background(), snap.WithValue("MC_LEVEL", "x")); err != nil {
				t.Fatal(err)
			}

			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != perm {
				t.Errorf("パーミッションが %v に変わった。%v のままであること", got, perm)
			}
		})
	}
}

// README は vi .env による手編集を正規の手順として案内している。
// 読み込んでから書き戻すまでの間に人間が編集していたら、上書きしてはいけない。
func TestStoreDetectsExternalModification(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)

	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// 外部から編集する（mtime の粒度に依存しないよう内容とサイズを変える）
	external := append(loadFixture(t), []byte("\nEXTERNALLY_ADDED=1\n")...)
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}

	err = store.Save(context.Background(), snap.WithValue("MC_LEVEL", "creative"))
	if !errors.Is(err, dotenv.ErrConflict) {
		t.Fatalf("ErrConflict を期待したが %v", err)
	}

	// 外部の編集が残っていること（上書きされていない）
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(after), "EXTERNALLY_ADDED=1") {
		t.Error("外部の編集が上書きされた")
	}
	if strings.Contains(string(after), "MC_LEVEL=creative") {
		t.Error("衝突したのに書き込まれている")
	}
}

// 一時ファイルが残らないこと。残ると次回の書き込みや ls が汚れる。
func TestStoreLeavesNoTempFiles(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), snap.WithValue("MC_LEVEL", "x")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".env" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("一時ファイルが残っている: %v", names)
	}
}

// 存在しない .env は明確なエラーにする。
// systemd 配下では作業ディレクトリが違うため、この失敗は起きやすい。
func TestStoreLoadMissingFile(t *testing.T) {
	t.Parallel()

	store := dotenv.NewFileStore(filepath.Join(t.TempDir(), ".env"))
	if _, err := store.Load(context.Background()); err == nil {
		t.Error("エラーになるはず")
	}
}

// Load で得た内容をそのまま Save しても、内容は 1 バイトも変わらない。
func TestStoreSaveWithoutChangesIsNoop(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), snap); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("読んで書き戻しただけで内容が変わった")
	}
}

// キャンセルされた context では書き込まない。
func TestStoreRespectsContext(t *testing.T) {
	t.Parallel()

	store, path := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Save(ctx, snap.WithValue("MC_LEVEL", "x")); !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled を期待したが %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("キャンセルされたのに書き込まれた")
	}
}

// Snapshot は不変。WithValue は新しい Snapshot を返し、元は変わらない。
func TestSnapshotIsImmutable(t *testing.T) {
	t.Parallel()

	store, _ := newStore(t, 0o600)
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	original, _ := snap.File().Get("MC_LEVEL")
	updated := snap.WithValue("MC_LEVEL", "creative")

	if got, _ := snap.File().Get("MC_LEVEL"); got != original {
		t.Errorf("元の Snapshot が変更された: %q -> %q", original, got)
	}
	if got, _ := updated.File().Get("MC_LEVEL"); got != "creative" {
		t.Errorf("新しい Snapshot に反映されていない: %q", got)
	}
	if snap.LoadedAt().IsZero() {
		t.Error("LoadedAt が設定されていない")
	}
	_ = time.Now()
}
