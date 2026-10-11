package backupfs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
)

/*
newPaperProject は Paper が実際に書く置き場所で data/ を作る。

ゲームルール・天候・生成設定はディメンション側にある。これが
Paper とバニラの違いそのもので、変換の対象になる。
*/
func newPaperProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	data := filepath.Join(root, "data")
	world := filepath.Join(data, "main")
	overworld := filepath.Join(world, "dimensions", "minecraft", "overworld")
	nether := filepath.Join(world, "dimensions", "minecraft", "the_nether")

	// ワールド直下
	writeFile(t, filepath.Join(world, "level.dat"), "level")
	writeFile(t, filepath.Join(world, "level.dat_old"), "level-old")
	writeFile(t, filepath.Join(world, "icon.png"), "icon")
	writeFile(t, filepath.Join(world, "session.lock"), "lock")
	writeFile(t, filepath.Join(world, "data", "minecraft", "scoreboard.dat"), "score")
	writeFile(t, filepath.Join(world, "players", "data", "abc.dat"), "player")

	// Paper がディメンション側に置くもの
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "world_gen_settings.dat"), "overworld-gen")
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "game_rules.dat"), "rules")
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "weather.dat"), "weather")
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "scheduled_events.dat"), "events")
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "wandering_trader.dat"), "trader")
	// バニラもディメンション側に置くもの
	writeFile(t, filepath.Join(overworld, "data", "minecraft", "raids.dat"), "raids")
	writeFile(t, filepath.Join(overworld, "region", "r.0.0.mca"), "region")
	// Paper 固有。バニラは読まない
	writeFile(t, filepath.Join(overworld, "data", "paper", "metadata.dat"), "paper-meta")
	writeFile(t, filepath.Join(overworld, "paper-world.yml"), "paper-yml")
	// ネザー側。移さずそのまま持ち出す
	writeFile(t, filepath.Join(nether, "data", "minecraft", "world_gen_settings.dat"), "nether-gen")

	// サーバーの設定。シングルプレイには持ち出さない
	writeFile(t, filepath.Join(data, "plugins", "Essentials.jar"), "plugin")
	writeFile(t, filepath.Join(data, "config", "paper-global.yml"), "config")
	writeFile(t, filepath.Join(data, "bukkit.yml"), "bukkit")
	writeFile(t, filepath.Join(data, "spigot.yml"), "spigot")

	return root
}

// createPaperBackup は Paper 形式のアーカイブを 1 件作る。
func createPaperBackup(t *testing.T, store *backupfs.Store) backup.ID {
	t.Helper()

	id := mustID(t, "backup-26.3-main-20261011-181445.zip")
	if _, err := store.Create(context.Background(), id, mustName(t, "main"), nil); err != nil {
		t.Fatal(err)
	}
	return id
}

// openedNames は開いたアーカイブのエントリ名を並べて返す。
func openedNames(t *testing.T, file port.ArchiveFile) []string {
	t.Helper()

	body, err := io.ReadAll(file.Body)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

// openedBody は開いたアーカイブの 1 エントリの中身を返す。
func openedBody(t *testing.T, file port.ArchiveFile, entry string) string {
	t.Helper()

	body, err := io.ReadAll(file.Body)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range r.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(got)
	}
	t.Fatalf("%q が無い", entry)
	return ""
}

/*
OpenVanilla はシングルプレイで読める形に包み直して開く。

出来上がりは saves/ にそのまま展開できる形、つまりワールドの
ディレクトリが zip の直下にある形になる。
*/
func TestOpenVanillaProducesSingleplayerLayout(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatalf("包み直せなかった: %v", err)
	}
	defer func() { _ = file.Body.Close() }()

	want := []string{
		// Paper がディメンション側に置いていたものが、ワールド直下へ移る
		"main/data/minecraft/game_rules.dat",
		"main/data/minecraft/scheduled_events.dat",
		"main/data/minecraft/scoreboard.dat",
		"main/data/minecraft/wandering_trader.dat",
		"main/data/minecraft/weather.dat",
		"main/data/minecraft/world_gen_settings.dat",
		// バニラもディメンション側に置くものは動かない
		"main/dimensions/minecraft/overworld/data/minecraft/raids.dat",
		"main/dimensions/minecraft/overworld/data/paper/metadata.dat",
		"main/dimensions/minecraft/overworld/paper-world.yml",
		"main/dimensions/minecraft/overworld/region/r.0.0.mca",
		"main/dimensions/minecraft/the_nether/data/minecraft/world_gen_settings.dat",
		"main/icon.png",
		"main/level.dat",
		"main/level.dat_old",
		"main/players/data/abc.dat",
	}

	if got := openedNames(t, file); !slices.Equal(got, want) {
		t.Errorf("エントリが違う。\n得た:   %v\nはず: %v", got, want)
	}
}

/*
ワールド直下の world_gen_settings.dat はオーバーワールドのものであること。

オーバーワールド側は 3 つのディメンションすべての設定を持っている。
ネザー側のものを持ってくると、全体の設定が 1 ディメンション分に縮む。
*/
func TestOpenVanillaTakesWorldGenSettingsFromOverworld(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	if got := openedBody(t, file, "main/data/minecraft/world_gen_settings.dat"); got != "overworld-gen" {
		t.Errorf("中身が %q。オーバーワールドの %q のはず", got, "overworld-gen")
	}
}

// 移すのは置き場所だけ。中身は 1 バイトも変えない。
func TestOpenVanillaPreservesContents(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	if got := openedBody(t, file, "main/level.dat"); got != "level" {
		t.Errorf("level.dat の中身が %q", got)
	}
}

// session.lock は持ち出さない。取得の時点で除いてある。
func TestOpenVanillaExcludesSessionLock(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	if slices.Contains(openedNames(t, file), "main/session.lock") {
		t.Error("session.lock が含まれている")
	}
}

/*
Range 要求に応えるため、Seek できる形で返す。

シングルプレイ用の zip も 200MB 規模になる。転送が途中で切れたときに
最初からやり直させない（REQ-415）。
*/
func TestOpenVanillaIsSeekable(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	if _, err := file.Body.Seek(0, io.SeekEnd); err != nil {
		t.Fatalf("末尾へ Seek できない: %v", err)
	}
	if _, err := file.Body.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("先頭へ戻せない: %v", err)
	}
	if file.Info.SizeBytes <= 0 {
		t.Errorf("大きさが %d。Content-Length に使えない", file.Info.SizeBytes)
	}
}

/*
包み直したものは保管先に残さない。

保管しているのは Paper 形式のアーカイブだけで、シングルプレイ用は
渡すためにその場で作るもの。残すと、一覧に出ない容量が積もっていく。
*/
func TestOpenVanillaLeavesNothingInBackupDir(t *testing.T) {
	t.Parallel()

	root := newPaperProject(t)
	store := newStore(t, root)
	id := createPaperBackup(t, store)
	backupDir := filepath.Join(root, "backups")

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	// 開いている間も、保管先には元のアーカイブしか見えない。
	if got := dirNames(t, backupDir); !slices.Equal(got, []string{id.String()}) {
		t.Errorf("開いている間の保管先が %v。%v だけのはず", got, []string{id.String()})
	}

	if err := file.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if got := dirNames(t, backupDir); !slices.Equal(got, []string{id.String()}) {
		t.Errorf("閉じた後の保管先が %v。%v だけのはず", got, []string{id.String()})
	}
}

// 失敗したときも残骸を残さない。
func TestOpenVanillaLeavesNothingOnFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := newStore(t, root)
	backupDir := filepath.Join(root, "backups")

	// level.dat の無いアーカイブを保管先に置く。
	id := mustID(t, "backup-26.3-main-20261011-181445.zip")
	writeZipFile(t, filepath.Join(backupDir, id.String()), map[string]string{
		"data/main/dimensions/minecraft/overworld/region/r.0.0.mca": "region",
	})

	if _, err := store.OpenVanilla(context.Background(), id); err == nil {
		t.Fatal("level.dat が無いのに包み直した")
	}

	if got := dirNames(t, backupDir); !slices.Equal(got, []string{id.String()}) {
		t.Errorf("保管先が %v。%v だけのはず", got, []string{id.String()})
	}
}

// 無い ID は ErrNotFound。Open と同じ扱いにする。
func TestOpenVanillaNotFound(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))

	_, err := store.OpenVanilla(context.Background(), mustID(t, "missing.zip"))
	if !errors.Is(err, backup.ErrNotFound) {
		t.Errorf("err = %v。ErrNotFound のはず", err)
	}
}

// 保管しているアーカイブには手を触れない。
func TestOpenVanillaKeepsStoredArchiveUntouched(t *testing.T) {
	t.Parallel()

	root := newPaperProject(t)
	store := newStore(t, root)
	id := createPaperBackup(t, store)
	path := filepath.Join(root, "backups", id.String())

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Body.Close()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("保管しているアーカイブが変わっている")
	}
}

// dirNames はディレクトリ直下の名前を並べて返す。
func dirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// writeZipFile は指定した場所に zip を作る。
func writeZipFile(t *testing.T, path string, files map[string]string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

/*
更新時刻は保管しているアーカイブのものを返すこと。

この値は Last-Modified になり、Range で再開するときの If-Range の
照合に使われる。包み直した一時ファイルの時刻を返すと、再開のために
もう一度来た要求で包み直しが走って時刻が変わり、照合が外れる。
http.ServeContent は Range を無視して 200 で全体を返し直すので、
200MB の転送が切れるたびに最初からやり直すことになる（REQ-135）。
*/
func TestOpenVanillaReportsStoredArchiveModTime(t *testing.T) {
	t.Parallel()

	root := newPaperProject(t)
	store := newStore(t, root)
	id := createPaperBackup(t, store)

	stored, err := os.Stat(filepath.Join(root, "backups", id.String()))
	if err != nil {
		t.Fatal(err)
	}

	file, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()

	if !file.Info.CreatedAt.Equal(stored.ModTime()) {
		t.Errorf("更新時刻が %v。保管しているアーカイブの %v のはず",
			file.Info.CreatedAt, stored.ModTime())
	}
}

// 何度開いても同じ更新時刻になること。Range の再開は同じ URL へ
// もう一度来るので、ここが揺れると続きから取れない。
func TestOpenVanillaModTimeIsStableAcrossCalls(t *testing.T) {
	t.Parallel()

	store := newStore(t, newPaperProject(t))
	id := createPaperBackup(t, store)

	first, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()

	second, err := store.OpenVanilla(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Body.Close() }()

	if !first.Info.CreatedAt.Equal(second.Info.CreatedAt) {
		t.Errorf("1 回目 %v と 2 回目 %v で更新時刻が違う",
			first.Info.CreatedAt, second.Info.CreatedAt)
	}
	// 大きさも揺れないこと。Content-Length に使う。
	if first.Info.SizeBytes != second.Info.SizeBytes {
		t.Errorf("1 回目 %d バイトと 2 回目 %d バイトで大きさが違う",
			first.Info.SizeBytes, second.Info.SizeBytes)
	}
}
