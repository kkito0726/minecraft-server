package worldfs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/filesystem/worldfs"
)

func name(t *testing.T, s string) world.Name {
	t.Helper()

	n, err := world.NewName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// newRepo は data/ を模したディレクトリと Repository を用意する。
func newRepo(t *testing.T, files map[string]string) (*worldfs.Repository, string) {
	t.Helper()

	dataDir := filepath.Join(t.TempDir(), "data")
	for rel, body := range files {
		p := filepath.Join(dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := worldfs.New(worldfs.Config{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	return repo, dataDir
}

func TestListSkipsNonWorlds(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{
		"world/level.dat":    "w",
		"creative/level.dat": "c",
		// data/ 直下の予約ディレクトリはワールドではない
		"logs/latest.log":     "l",
		"plugins/spark/x.yml": "p",
		"config/paper.yml":    "c",
		"cache/x":             "x",
		"libraries/a/b.jar":   "j",
		"versions/26.2/x.jar": "v",
		// 直下のファイルもワールドではない
		"server.properties": "s",
		"spigot.yml":        "s",
	})
	// 退避ディレクトリもワールドとして扱わない
	if err := os.MkdirAll(filepath.Join(dataDir, "world.broken-20260101-000000"), 0o755); err != nil {
		t.Fatal(err)
	}

	worlds, err := repo.List(context.Background(), name(t, "world"))
	if err != nil {
		t.Fatalf("List に失敗: %v", err)
	}

	got := map[string]bool{}
	for _, w := range worlds {
		got[w.Name().String()] = true
	}
	if len(got) != 2 || !got["world"] || !got["creative"] {
		t.Errorf("一覧が %v。world と creative だけのはず", got)
	}
}

// 稼働中のワールドに印が付く。
func TestListMarksActive(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{
		"world/level.dat":    "w",
		"creative/level.dat": "c",
	})

	worlds, err := repo.List(context.Background(), name(t, "creative"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range worlds {
		want := w.Name().String() == "creative"
		if w.IsActive() != want {
			t.Errorf("%s の IsActive が %v", w.Name(), w.IsActive())
		}
	}
}

// session.lock の有無を拾う。停止時に消えないので異常ではない。
func TestListDetectsSessionLock(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{
		"world/level.dat":    "w",
		"world/session.lock": "",
		"creative/level.dat": "c",
	})

	worlds, err := repo.List(context.Background(), name(t, "world"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range worlds {
		want := w.Name().String() == "world"
		if w.HasSessionLock() != want {
			t.Errorf("%s の HasSessionLock が %v", w.Name(), w.HasSessionLock())
		}
	}
}

func TestExists(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{"world/level.dat": "w"})
	ctx := context.Background()

	if ok, err := repo.Exists(ctx, name(t, "world")); err != nil || !ok {
		t.Errorf("world があるはず: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Exists(ctx, name(t, "nope")); err != nil || ok {
		t.Errorf("nope は無いはず: ok=%v err=%v", ok, err)
	}
}

// 複製は session.lock を含めない。起動時に作り直されるため。
func TestCopyExcludesSessionLock(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{
		"world/level.dat":               "level",
		"world/session.lock":            "lock",
		"world/dimensions/nether/r.mca": "region",
		"world/players/uuid.dat":        "player",
	})

	if err := repo.Copy(context.Background(), name(t, "world"), name(t, "backup"), nil); err != nil {
		t.Fatalf("Copy に失敗: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "backup/session.lock")); !os.IsNotExist(err) {
		t.Error("session.lock が複製されている")
	}
	for _, rel := range []string{"level.dat", "dimensions/nether/r.mca", "players/uuid.dat"} {
		if _, err := os.Stat(filepath.Join(dataDir, "backup", rel)); err != nil {
			t.Errorf("%s が複製されていない: %v", rel, err)
		}
	}
}

// 複製先が既にあれば拒否する。既存ワールドの意図しない上書きを防ぐ。
func TestCopyRejectsExistingDestination(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{
		"world/level.dat":    "w",
		"creative/level.dat": "c",
	})

	err := repo.Copy(context.Background(), name(t, "world"), name(t, "creative"), nil)
	if !errors.Is(err, worldfs.ErrAlreadyExists) {
		t.Errorf("ErrAlreadyExists を期待したが %v", err)
	}
}

// 複製の途中で失敗したら、中途半端なディレクトリを残さない。
func TestCopyCleansUpOnFailure(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/level.dat": "w"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.Copy(ctx, name(t, "world"), name(t, "backup"), nil); err == nil {
		t.Fatal("エラーになるはず")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backup")); !os.IsNotExist(err) {
		t.Error("中途半端なディレクトリが残っている")
	}
}

func TestRename(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/level.dat": "w"})

	if err := repo.Rename(context.Background(), name(t, "world"), name(t, "renamed")); err != nil {
		t.Fatalf("Rename に失敗: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "renamed/level.dat")); err != nil {
		t.Errorf("改名先が無い: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "world")); !os.IsNotExist(err) {
		t.Error("改名元が残っている")
	}
}

func TestRenameRejectsExistingDestination(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{
		"world/level.dat":    "w",
		"creative/level.dat": "c",
	})

	err := repo.Rename(context.Background(), name(t, "world"), name(t, "creative"))
	if !errors.Is(err, worldfs.ErrAlreadyExists) {
		t.Errorf("ErrAlreadyExists を期待したが %v", err)
	}
}

// 退避は削除ではなく移動。復元に失敗しても戻せるようにするため。
func TestQuarantineMovesNotDeletes(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/marker.txt": "残っていること"})
	ctx := context.Background()

	q, err := repo.Quarantine(ctx, name(t, "world"), world.QuarantineFromRestore)
	if err != nil {
		t.Fatalf("Quarantine に失敗: %v", err)
	}

	// 元の場所は空になる
	if _, err := os.Stat(filepath.Join(dataDir, "world")); !os.IsNotExist(err) {
		t.Error("元のディレクトリが残っている")
	}
	// 退避先に中身がそのままある
	body, err := os.ReadFile(filepath.Join(dataDir, q.DirName(), "marker.txt"))
	if err != nil {
		t.Fatalf("退避先が読めない: %v", err)
	}
	if string(body) != "残っていること" {
		t.Errorf("内容が %q", body)
	}
	if !strings.Contains(q.DirName(), ".broken-") {
		t.Errorf("退避名が %q", q.DirName())
	}
}

// 退避したものを元に戻せる。復元の展開に失敗したときのロールバック。
func TestRestoreQuarantine(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/marker.txt": "元の内容"})
	ctx := context.Background()

	q, err := repo.Quarantine(ctx, name(t, "world"), world.QuarantineFromRestore)
	if err != nil {
		t.Fatal(err)
	}
	// 展開途中のディレクトリがある状態を再現
	if err := os.MkdirAll(filepath.Join(dataDir, "world"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := repo.Restore(ctx, q, name(t, "world")); err != nil {
		t.Fatalf("Restore に失敗: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dataDir, "world/marker.txt"))
	if err != nil {
		t.Fatalf("戻っていない: %v", err)
	}
	if string(body) != "元の内容" {
		t.Errorf("内容が %q", body)
	}
	if _, err := os.Stat(filepath.Join(dataDir, q.DirName())); !os.IsNotExist(err) {
		t.Error("退避先が残っている")
	}
}

func TestListQuarantines(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/level.dat": "w"})
	for _, dir := range []string{
		"world.broken-20260909-143000",
		"creative.deleted-20260908-120000",
		"notaquarantine",
	} {
		if err := os.MkdirAll(filepath.Join(dataDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListQuarantines(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("退避が %d 件。2 件のはず", len(list))
	}

	kinds := map[world.QuarantineKind]bool{}
	for _, q := range list {
		kinds[q.Kind()] = true
	}
	if !kinds[world.QuarantineFromRestore] || !kinds[world.QuarantineFromDelete] {
		t.Errorf("種類が %v", kinds)
	}
}

func TestRemoveQuarantine(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/marker.txt": "x"})
	ctx := context.Background()

	q, err := repo.Quarantine(ctx, name(t, "world"), world.QuarantineFromDelete)
	if err != nil {
		t.Fatal(err)
	}

	freed, err := repo.RemoveQuarantine(ctx, q)
	if err != nil {
		t.Fatalf("RemoveQuarantine に失敗: %v", err)
	}
	if freed <= 0 {
		t.Errorf("解放したサイズが %d", freed)
	}
	if _, err := os.Stat(filepath.Join(dataDir, q.DirName())); !os.IsNotExist(err) {
		t.Error("削除されていない")
	}
}

func TestRemove(t *testing.T) {
	t.Parallel()

	repo, dataDir := newRepo(t, map[string]string{"world/level.dat": "w"})

	if err := repo.Remove(context.Background(), name(t, "world")); err != nil {
		t.Fatalf("Remove に失敗: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "world")); !os.IsNotExist(err) {
		t.Error("削除されていない")
	}
}

// 空き容量が取れること。展開前の事前確認に使う。
func TestAvailableBytes(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{"world/level.dat": "w"})

	n, err := repo.AvailableBytes(context.Background())
	if err != nil {
		t.Fatalf("AvailableBytes に失敗: %v", err)
	}
	if n <= 0 {
		t.Errorf("空き容量が %d", n)
	}
}

// 存在しないワールドへの操作は明確なエラーにする。
func TestOperationsOnMissingWorld(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{"world/level.dat": "w"})
	ctx := context.Background()
	missing := name(t, "nope")

	if err := repo.Copy(ctx, missing, name(t, "dst"), nil); !errors.Is(err, worldfs.ErrNotFound) {
		t.Errorf("Copy: ErrNotFound を期待したが %v", err)
	}
	if err := repo.Rename(ctx, missing, name(t, "dst")); !errors.Is(err, worldfs.ErrNotFound) {
		t.Errorf("Rename: ErrNotFound を期待したが %v", err)
	}
	if _, err := repo.Quarantine(ctx, missing, world.QuarantineFromDelete); !errors.Is(err, worldfs.ErrNotFound) {
		t.Errorf("Quarantine: ErrNotFound を期待したが %v", err)
	}
}

// DataDir の外を指すパスは組み立てられない。
func TestNewRequiresDataDir(t *testing.T) {
	t.Parallel()

	if _, err := worldfs.New(worldfs.Config{}); err == nil {
		t.Error("DataDir が無ければエラーになるはず")
	}
}

// level.dat を読めないワールドも一覧には出る。
// 破損したワールドが 1 つあっても画面全体が使えなくなってはいけない。
func TestListIncludesUnreadableWorlds(t *testing.T) {
	t.Parallel()

	repo, _ := newRepo(t, map[string]string{
		"world/level.dat":  "これは NBT ではない",
		"broken/other.txt": "level.dat が無い",
	})

	worlds, err := repo.List(context.Background(), name(t, "world"))
	if err != nil {
		t.Fatalf("List に失敗: %v", err)
	}
	if len(worlds) != 2 {
		t.Fatalf("一覧が %d 件。2 件のはず", len(worlds))
	}
	for _, w := range worlds {
		if w.Version().Readable() {
			t.Errorf("%s のバージョンが読めた扱いになっている", w.Name())
		}
		if w.Version().Display() != "不明" {
			t.Errorf("%s の表示が %q", w.Name(), w.Version().Display())
		}
	}
}

var _ = time.Now
