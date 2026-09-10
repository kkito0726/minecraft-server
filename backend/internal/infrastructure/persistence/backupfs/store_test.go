package backupfs_test

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
)

// newProject は data/ の実物に近い一時ディレクトリを作る。
//
// バックアップ対象（5 項目）と非対象の双方を置く。非対象が
// 混入しないことを「集合の完全一致」で検証するため、両方が要る。
func newProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	data := filepath.Join(root, "data")

	// バックアップ対象
	writeFile(t, filepath.Join(data, "world", "level.dat"), "level")
	writeFile(t, filepath.Join(data, "world", "dimensions", "minecraft", "overworld", "r.0.0.mca"), "region")
	writeFile(t, filepath.Join(data, "world", "session.lock"), "lock")
	writeFile(t, filepath.Join(data, "plugins", "Essentials.jar"), "plugin")
	writeFile(t, filepath.Join(data, "config", "paper-global.yml"), "config")
	writeFile(t, filepath.Join(data, "bukkit.yml"), "bukkit")
	writeFile(t, filepath.Join(data, "spigot.yml"), "spigot")

	// バックアップ対象ではないもの
	writeFile(t, filepath.Join(data, "server.properties"), "rcon.password=ひみつ")
	writeFile(t, filepath.Join(data, "libraries", "lib.jar"), "lib")
	writeFile(t, filepath.Join(data, "versions", "v.json"), "version")
	writeFile(t, filepath.Join(data, "cache", "c.bin"), "cache")
	writeFile(t, filepath.Join(data, "logs", "latest.log"), "log")
	writeFile(t, filepath.Join(data, "paper-26.2-121.jar"), "jar")
	writeFile(t, filepath.Join(data, "ops.json"), "[]")
	writeFile(t, filepath.Join(data, "whitelist.json"), "[]")

	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newStore(t *testing.T, root string) *backupfs.Store {
	t.Helper()

	store, err := backupfs.New(backupfs.Config{
		ProjectDir: root,
		BackupDir:  filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustID(t *testing.T, name string) backup.ID {
	t.Helper()

	id, err := backup.NewID(name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustName(t *testing.T, name string) world.Name {
	t.Helper()

	n, err := world.NewName(name)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TC-004-02: アーカイブのルート集合が網羅列挙と完全一致する。
//
// 「非対象が入らない」を個別に確かめるのではなく集合の一致で見る。
// こうすると、将来 data/ に新しいパスが増えたときの意図しない混入も捕まる。
func TestCreateArchivesExactlyTheBackupSet(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}

	got := rootsOf(t, filepath.Join(root, "backups", id.String()))
	want := []string{
		"data/bukkit.yml", "data/config", "data/plugins", "data/spigot.yml", "data/world",
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("ルート集合が %v。%v のはず", got, want)
	}
}

// server.properties は rcon.password と management-server-secret を平文で持つ。
// アーカイブに入れると、バックアップを渡した相手にサーバーの操作権を渡すことになる。
func TestCreateNeverIncludesServerProperties(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}

	for _, name := range entriesOf(t, filepath.Join(root, "backups", id.String())) {
		if strings.Contains(name, "server.properties") {
			t.Fatalf("アーカイブに %q が含まれている", name)
		}
	}
}

// session.lock は稼働中のサーバーが掴んでいる。持ち込まない。
func TestCreateExcludesSessionLock(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}

	if slices.Contains(entriesOf(t, filepath.Join(root, "backups", id.String())), "data/world/session.lock") {
		t.Error("session.lock が含まれている")
	}
}

// TC-004-05: 対象のワールドは引数に従う。world を固定値にしない。
func TestCreateUsesGivenLevel(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	writeFile(t, filepath.Join(root, "data", "creative", "level.dat"), "creative")
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-creative-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "creative"), nil); err != nil {
		t.Fatal(err)
	}

	roots := rootsOf(t, filepath.Join(root, "backups", id.String()))
	if !slices.Contains(roots, "data/creative") {
		t.Errorf("ルートが %v。data/creative を含むはず", roots)
	}
	if slices.Contains(roots, "data/world") {
		t.Errorf("ルートが %v。data/world を含まないはず", roots)
	}
}

// spigot.yml が無い環境でも取得できる。
// バックアップ対象は網羅列挙だが、実在しないものは飛ばす。
func TestCreateSkipsMissingExtras(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	if err := os.Remove(filepath.Join(root, "data", "spigot.yml")); err != nil {
		t.Fatal(err)
	}
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatalf("spigot.yml が無いと取得できない: %v", err)
	}

	if slices.Contains(rootsOf(t, filepath.Join(root, "backups", id.String())), "data/spigot.yml") {
		t.Error("存在しないはずの spigot.yml が含まれている")
	}
}

// 対象のワールドが存在しなければ取得しない。
// 空のアーカイブを「バックアップが取れた」として残すと復元で全損する。
func TestCreateFailsWhenLevelMissing(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-nothing-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "nothing"), nil); err == nil {
		t.Fatal("存在しないワールドで取得が成功した")
	}
	if _, err := os.Stat(filepath.Join(root, "backups", id.String())); !os.IsNotExist(err) {
		t.Error("失敗したのにアーカイブが残っている")
	}
}

// TC-004-07: 保管先が無ければ作る。
func TestCreateMakesBackupDirectory(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	if _, err := os.Stat(filepath.Join(root, "backups")); !os.IsNotExist(err) {
		t.Fatal("前提: backups/ は存在しないはず")
	}

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "backups", id.String())); err != nil {
		t.Errorf("アーカイブが作られていない: %v", err)
	}
}

// 取得したアーカイブは実際に展開できる。
func TestCreateProducesExtractableArchive(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	stored, err := store.Create(context.Background(), id, mustName(t, "world"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SizeBytes <= 0 {
		t.Errorf("サイズが %d バイト", stored.SizeBytes)
	}

	entries := entriesOf(t, filepath.Join(root, "backups", id.String()))
	for _, want := range []string{
		"data/world/level.dat",
		"data/world/dimensions/minecraft/overworld/r.0.0.mca",
		"data/plugins/Essentials.jar",
		"data/config/paper-global.yml",
		"data/bukkit.yml",
		"data/spigot.yml",
	} {
		if !slices.Contains(entries, want) {
			t.Errorf("%q が含まれていない。実際: %v", want, entries)
		}
	}
}

// 進捗が通知される。
func TestCreateReportsProgress(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	var calls int
	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	_, err := store.Create(context.Background(), id, mustName(t, "world"),
		func(int64, int64) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("進捗が 1 度も通知されていない")
	}
}

// TC-004-E02: 作成途中の一時ファイルは一覧に現れない。
func TestListSkipsPartialFiles(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	dir := filepath.Join(root, "backups")
	writeFile(t, filepath.Join(dir, "backup-26.2-world-20260901-000000.zip"), "zip")
	writeFile(t, filepath.Join(dir, "backup-26.2-world-20260902-000000.zip.part-123456"), "partial")
	writeFile(t, filepath.Join(dir, "notes.txt"), "メモ")
	if err := os.MkdirAll(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := newStore(t, root).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d 件: %v", len(got), namesOf(got))
	}
	if got[0].ID.String() != "backup-26.2-world-20260901-000000.zip" {
		t.Errorf("%q", got[0].ID)
	}
}

// 保管先が存在しなくても一覧はエラーにならない。まだ 1 度も取っていない状態。
func TestListOnMissingDirectory(t *testing.T) {
	t.Parallel()

	got, err := newStore(t, newProject(t)).List(context.Background())
	if err != nil {
		t.Fatalf("保管先が無いだけでエラーになる: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d 件", len(got))
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	path := filepath.Join(root, "backups", "backup-26.2-world-20260901-000000.zip")
	writeFile(t, path, "0123456789")

	freed, err := newStore(t, root).Delete(
		context.Background(), mustID(t, "backup-26.2-world-20260901-000000.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if freed != 10 {
		t.Errorf("解放が %d バイト", freed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("削除されていない")
	}
}

func TestDeleteMissing(t *testing.T) {
	t.Parallel()

	_, err := newStore(t, newProject(t)).Delete(
		context.Background(), mustID(t, "backup-26.2-world-20260901-000000.zip"))
	if !errors.Is(err, backup.ErrNotFound) {
		t.Errorf("エラーが %v。ErrNotFound のはず", err)
	}
}

// アーカイブの構成は展開せずに読める。
func TestInspect(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}

	info, err := store.Inspect(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if info.Level != "world" {
		t.Errorf("Level が %q", info.Level)
	}
	if !info.HasLevelDat {
		t.Error("level.dat が見つかっていない")
	}
	if info.TotalBytes <= 0 {
		t.Errorf("TotalBytes が %d", info.TotalBytes)
	}
	if !slices.Contains(info.EntryRoots, "data/world") {
		t.Errorf("EntryRoots が %v", info.EntryRoots)
	}
}

// level.dat は zip 全体を展開せずに読める。バージョン判定の前提。
func TestOpenLevelDat(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	id := mustID(t, "backup-26.2-world-20260910-143000.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "world"), nil); err != nil {
		t.Fatal(err)
	}

	rc, err := store.OpenLevelDat(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "level" {
		t.Errorf("中身が %q", content)
	}
}

// level.dat を含まないアーカイブでも壊れない。
func TestOpenLevelDatMissing(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	store := newStore(t, root)

	// ワールドを含まないアーカイブを手で作る
	dir := filepath.Join(root, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(dir, "backup-26.2-world-20260901-000000.zip"),
		map[string]string{"data/bukkit.yml": "bukkit"})

	id := mustID(t, "backup-26.2-world-20260901-000000.zip")
	if _, err := store.OpenLevelDat(context.Background(), id); !errors.Is(err, backup.ErrNotFound) {
		t.Errorf("エラーが %v。ErrNotFound のはず", err)
	}

	info, err := store.Inspect(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasLevelDat {
		t.Error("level.dat があることになっている")
	}
}

func TestDirectory(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	if got := newStore(t, root).Directory(); got != filepath.Join(root, "backups") {
		t.Errorf("Directory() = %q", got)
	}
}

// 保管先は必須。
func TestNewRequiresDirectories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  backupfs.Config
	}{
		{name: "プロジェクトディレクトリなし", cfg: backupfs.Config{BackupDir: "/tmp/b"}},
		{name: "保管先なし", cfg: backupfs.Config{ProjectDir: "/tmp/p"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := backupfs.New(tt.cfg); err == nil {
				t.Error("エラーにならない")
			}
		})
	}
}

// 更新時刻が読み取れる。世代管理の判定に使う。
func TestListReadsModTime(t *testing.T) {
	t.Parallel()

	root := newProject(t)
	path := filepath.Join(root, "backups", "backup-26.2-world-20260901-000000.zip")
	writeFile(t, path, "zip")

	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := newStore(t, root).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diff := got[0].CreatedAt.Sub(past); diff > time.Second || diff < -time.Second {
		t.Errorf("CreatedAt が %v。%v のはず", got[0].CreatedAt, past)
	}
}

func rootsOf(t *testing.T, path string) []string {
	t.Helper()

	seen := map[string]bool{}
	var roots []string
	for _, name := range entriesOf(t, path) {
		rest, ok := strings.CutPrefix(name, "data/")
		if !ok {
			t.Fatalf("data/ 以外のエントリ: %q", name)
		}
		root := "data/" + strings.SplitN(rest, "/", 2)[0]
		if !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}

func entriesOf(t *testing.T, path string) []string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	var names []string
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		names = append(names, f.Name)
	}
	return names
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func namesOf(entries []port.StoredBackup) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID.String())
	}
	return out
}
