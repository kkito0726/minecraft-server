package backupfs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newImportStore は取り込みの試験用に、保管先のパスも一緒に返す。
func newImportStore(t *testing.T) (*backupfs.Store, string) {
	t.Helper()

	project := t.TempDir()
	return newStore(t, project), filepath.Join(project, "backups")
}

/*
取り込みの途中にある一時ファイルが一覧に現れてはいけない。

List は「.zip で終わるもの」をアーカイブとみなすので、一時ファイルに
拡張子を付けると、アップロード中に壊れたアーカイブが 1 件見えることに
なる。しかもそれを復元しようとすると失敗する。
*/
func TestStagedArchiveIsNotListed(t *testing.T) {
	t.Parallel()

	store, _ := newImportStore(t)
	body := zipBytes(t, map[string]string{"data/world/level.dat": "nbt"})

	staged, err := store.Stage(context.Background(), bytes.NewReader(body), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("取り込みの途中で一覧に現れている: %v", got)
	}
}

func TestStageThenAdopt(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)
	body := zipBytes(t, map[string]string{"data/world/level.dat": "nbt"})

	staged, err := store.Stage(context.Background(), bytes.NewReader(body), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	if entries := staged.LevelDatEntries(); len(entries) != 1 {
		t.Fatalf("level.dat を見つけられていない: %v", entries)
	}

	id, err := backup.NewID("backup-26.2-world-20260911-010000-imported.zip")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := staged.Adopt(context.Background(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SizeBytes == 0 {
		t.Error("大きさが 0")
	}

	// 確定したら一時ファイルは残らない。
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Name() != id.String() {
		t.Errorf("保管先に余計なものが残っている: %v", names(left))
	}
}

// 配布ワールドの形は data/ 配下へ包み直してから置く。
func TestAdoptRewrapsIntoData(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)
	body := zipBytes(t, map[string]string{"MyWorld/level.dat": "nbt"})

	staged, err := store.Stage(context.Background(), bytes.NewReader(body), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	id, err := backup.NewID("backup-unknown-MyWorld-20260911-010000-imported.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := staged.Adopt(context.Background(), id, "data/"); err != nil {
		t.Fatal(err)
	}

	info, err := store.Inspect(context.Background(), id)
	if err != nil {
		t.Fatalf("包み直した結果を読めない: %v", err)
	}
	if info.Level != "MyWorld" {
		t.Errorf("ワールド名が %q", info.Level)
	}

	// 包み直しに使った中間ファイルも残らない。
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 {
		t.Errorf("中間ファイルが残っている: %v", names(left))
	}
}

// 既にあるアーカイブを取り込みで失うことがあってはならない。
func TestAdoptDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := backup.NewID("backup-26.2-world-20260911-010000-imported.zip")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id.String()), []byte("既存"), 0o600); err != nil {
		t.Fatal(err)
	}

	staged, err := store.Stage(context.Background(),
		bytes.NewReader(zipBytes(t, map[string]string{"data/world/level.dat": "nbt"})), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()

	if _, err := staged.Adopt(context.Background(), id, ""); !errors.Is(err, os.ErrExist) {
		t.Fatalf("上書きされた、または別のエラー: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, id.String()))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "既存" {
		t.Error("既存のアーカイブが壊れた")
	}
}

// Pi ではディスクを埋めきるとサーバーごと止まる。上限で打ち切る。
func TestStageStopsAtLimit(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)
	huge := strings.NewReader(strings.Repeat("x", 4096))

	if _, err := store.Stage(context.Background(), huge, 1024); !errors.Is(err, backupfs.ErrTooLarge) {
		t.Fatalf("上限を超えても受け取った: %v", err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("打ち切ったのに残骸がある: %v", names(left))
	}
}

// zip として読めないものは受け取らない。
func TestStageRejectsNonZip(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)

	if _, err := store.Stage(context.Background(),
		strings.NewReader("これは zip ではない"), 1<<20); err == nil {
		t.Fatal("受理された")
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("拒否したのに残骸がある: %v", names(left))
	}
}

// 捨てたら何も残らない。中断がゴミを積むと、いずれディスクが埋まる。
func TestDiscardRemovesStagedFile(t *testing.T) {
	t.Parallel()

	store, dir := newImportStore(t)
	staged, err := store.Stage(context.Background(),
		bytes.NewReader(zipBytes(t, map[string]string{"data/world/level.dat": "nbt"})), 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	staged.Discard()

	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("捨てたのに残っている: %v", names(left))
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
