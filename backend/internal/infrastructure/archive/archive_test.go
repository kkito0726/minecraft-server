package archive_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

// 展開先に置いてよいのは data/ 配下だけ。これを緩めると、
// アーカイブの中身次第でリポジトリの外にファイルを書ける。
func TestInspectRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry string
	}{
		{"親ディレクトリ", "../../etc/passwd"},
		{"絶対パス", "/etc/passwd"},
		{"data の外", "etc/passwd"},
		{"data からの脱出", "data/../../tmp/x"},
		{"深い脱出", "data/world/../../../tmp/x"},
		{"Windows 形式の絶対パス", `C:\Windows\system32`},
		{"バックスラッシュでの脱出", `data\..\..\tmp\x`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeZip(t, map[string]string{tt.entry: "x"})

			_, err := archive.Inspect(context.Background(), path)
			if !errors.Is(err, archive.ErrUnsafeEntry) {
				t.Fatalf("ErrUnsafeEntry を期待したが %v", err)
			}
			// 拒否の理由に該当エントリ名が出ること。
			// エラーは %q で出すため、比較側も同じ形にする。
			quoted := strconv.Quote(tt.entry)
			if err != nil && !strings.Contains(err.Error(), quoted) {
				t.Errorf("エラーにエントリ名 %s が含まれていない: %v", quoted, err)
			}
		})
	}
}

// 危険なエントリは「展開を始める前に」弾く。
// 途中まで書いてから気づくと、中途半端なファイルが残る。
func TestExtractRejectsBeforeWriting(t *testing.T) {
	t.Parallel()

	// 安全なエントリの後に危険なエントリを置く
	path := writeZipOrdered(t, []zipEntry{
		{name: "data/world/level.dat", body: "ok"},
		{name: "../escape.txt", body: "bad"},
	})
	dest := t.TempDir()

	err := archive.Extract(context.Background(), path, dest, archive.ExtractOptions{}, nil)
	if !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Fatalf("ErrUnsafeEntry を期待したが %v", err)
	}

	// 1 バイトも書かれていないこと
	entries, readErr := os.ReadDir(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("展開先にファイルが残っている: %d 件", len(entries))
	}
}

// シンボリックリンクは展開しない。リンク先を経由して data/ の外へ書けるため。
func TestExtractRejectsSymlink(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "data/world/link"}
	header.SetMode(os.ModeSymlink | 0o777)
	f, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, "/etc/passwd"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "symlink.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := archive.Inspect(context.Background(), path); !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("ErrUnsafeEntry を期待したが %v", err)
	}
}

// 空のディレクトリも残す。data/world/datapacks は既定で空だが、
// docs の zip -qr は保持するため、往復させたときに差分を出さない。
func TestCreatePreservesEmptyDirectories(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, map[string]string{"data/world/level.dat": "level"})
	if err := os.MkdirAll(filepath.Join(src, "data/world/datapacks"), 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	sources := []archive.Source{{Root: src, Path: "data/world"}}
	if err := archive.Create(context.Background(), archivePath, sources, nil); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := archive.Extract(context.Background(), archivePath, dest, archive.ExtractOptions{}, nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dest, "data/world/datapacks"))
	if err != nil {
		t.Fatalf("空のディレクトリが復元されていない: %v", err)
	}
	if !info.IsDir() {
		t.Error("ディレクトリとして復元されていない")
	}
}

func TestCreateAndExtractRoundTrip(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"data/world/level.dat":               "level",
		"data/world/dimensions/nether/r.mca": "region",
		"data/plugins/config.yml":            "plugins",
		"data/spigot.yml":                    "spigot",
	})

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	sources := []archive.Source{
		{Root: src, Path: "data/world"},
		{Root: src, Path: "data/plugins"},
		{Root: src, Path: "data/spigot.yml"},
	}

	if err := archive.Create(context.Background(), archivePath, sources, nil); err != nil {
		t.Fatalf("Create に失敗: %v", err)
	}

	dest := t.TempDir()
	if err := archive.Extract(context.Background(), archivePath, dest, archive.ExtractOptions{}, nil); err != nil {
		t.Fatalf("Extract に失敗: %v", err)
	}

	for path, want := range map[string]string{
		"data/world/level.dat":               "level",
		"data/world/dimensions/nether/r.mca": "region",
		"data/plugins/config.yml":            "plugins",
		"data/spigot.yml":                    "spigot",
	} {
		got, err := os.ReadFile(filepath.Join(dest, path))
		if err != nil {
			t.Errorf("%s を読めない: %v", path, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s の内容が %q。%q のはず", path, got, want)
		}
	}
}

// 指定した接頭辞のファイルはアーカイブに含めない。
// session.lock は起動時に作り直されるので持ち込む意味がない。
func TestCreateExcludes(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"data/world/level.dat":    "level",
		"data/world/session.lock": "lock",
	})

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	sources := []archive.Source{
		{Root: src, Path: "data/world", ExcludeNames: []string{"session.lock"}},
	}
	if err := archive.Create(context.Background(), archivePath, sources, nil); err != nil {
		t.Fatal(err)
	}

	m, err := archive.Inspect(context.Background(), archivePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range m.EntryNames {
		if strings.HasSuffix(name, "session.lock") {
			t.Errorf("除外したはずの %s が含まれている", name)
		}
	}
}

// Inspect はアーカイブの構造を、全体を展開せずに読む。
func TestInspect(t *testing.T) {
	t.Parallel()

	path := writeZip(t, map[string]string{
		"data/world/level.dat":     "level",
		"data/world/players/a.dat": "player",
		"data/plugins/x.yml":       "plugin",
		"data/spigot.yml":          "spigot",
	})

	m, err := archive.Inspect(context.Background(), path)
	if err != nil {
		t.Fatalf("Inspect に失敗: %v", err)
	}

	if m.LevelDir != "world" {
		t.Errorf("LevelDir が %q。world のはず", m.LevelDir)
	}
	if m.LevelDatEntry != "data/world/level.dat" {
		t.Errorf("LevelDatEntry が %q", m.LevelDatEntry)
	}
	if m.EntryCount != 4 {
		t.Errorf("EntryCount が %d。4 のはず", m.EntryCount)
	}
	if m.TotalBytes <= 0 {
		t.Errorf("TotalBytes が %d", m.TotalBytes)
	}
	wantRoots := map[string]bool{"data/world": true, "data/plugins": true, "data/spigot.yml": true}
	for _, r := range m.Roots {
		if !wantRoots[r] {
			t.Errorf("想定外のルート %q", r)
		}
	}
}

// アーカイブ内の 1 エントリだけを、全体を展開せずに読む。
// 復元の事前確認で level.dat のバージョンを見るために使う。
func TestOpenEntry(t *testing.T) {
	t.Parallel()

	path := writeZip(t, map[string]string{
		"data/world/level.dat": "level-content",
		"data/big.bin":         strings.Repeat("x", 1024),
	})

	rc, err := archive.OpenEntry(context.Background(), path, "data/world/level.dat")
	if err != nil {
		t.Fatalf("OpenEntry に失敗: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "level-content" {
		t.Errorf("内容が %q", got)
	}
}

func TestOpenEntryMissing(t *testing.T) {
	t.Parallel()

	path := writeZip(t, map[string]string{"data/world/level.dat": "x"})

	if _, err := archive.OpenEntry(context.Background(), path, "data/nope.dat"); err == nil {
		t.Error("エラーになるはず")
	}
}

// 復元先のワールド名を変える場合、展開時にパスを書き換える。
// これが無いと、稼働中のワールドと別のディレクトリに戻ってしまい
// 「復元したのに何も変わらない」状態になる。
func TestExtractRewritesPrefix(t *testing.T) {
	t.Parallel()

	path := writeZip(t, map[string]string{
		"data/world/level.dat": "level",
		"data/plugins/x.yml":   "plugin",
	})
	dest := t.TempDir()

	opts := archive.ExtractOptions{
		RewritePrefix: map[string]string{"data/world/": "data/creative/"},
	}
	if err := archive.Extract(context.Background(), path, dest, opts, nil); err != nil {
		t.Fatalf("Extract に失敗: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "data/creative/level.dat")); err != nil {
		t.Errorf("書き換え先に展開されていない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "data/world/level.dat")); !os.IsNotExist(err) {
		t.Error("書き換え元にも展開されている")
	}
	// 対象外のエントリはそのまま
	if _, err := os.Stat(filepath.Join(dest, "data/plugins/x.yml")); err != nil {
		t.Errorf("対象外のエントリが展開されていない: %v", err)
	}
}

// 書き換えの結果も安全でなければならない。
func TestExtractRejectsUnsafeRewrite(t *testing.T) {
	t.Parallel()

	path := writeZip(t, map[string]string{"data/world/level.dat": "x"})
	opts := archive.ExtractOptions{
		RewritePrefix: map[string]string{"data/world/": "../escape/"},
	}

	err := archive.Extract(context.Background(), path, t.TempDir(), opts, nil)
	if !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("ErrUnsafeEntry を期待したが %v", err)
	}
}

// 進捗はバイト数で通知される。Pi では 30MB のワールドが数十秒かかる。
func TestProgressIsReported(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"data/world/a.dat": strings.Repeat("a", 4096),
		"data/world/b.dat": strings.Repeat("b", 4096),
	})

	var lastDone int64
	progress := func(done, _ int64) { lastDone = done }

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	sources := []archive.Source{{Root: src, Path: "data/world"}}
	if err := archive.Create(context.Background(), archivePath, sources, progress); err != nil {
		t.Fatal(err)
	}

	if lastDone < 8192 {
		t.Errorf("進捗の合計が %d。8192 以上のはず", lastDone)
	}
}

// キャンセルされた context では処理を止める。
func TestRespectsContext(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFiles(t, src, map[string]string{"data/world/a.dat": "x"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	sources := []archive.Source{{Root: src, Path: "data/world"}}

	if err := archive.Create(ctx, archivePath, sources, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled を期待したが %v", err)
	}
}

// 失敗したら中途半端なアーカイブを残さない。
func TestCreateLeavesNoPartialArchive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "backup.zip")

	// 存在しないソースを指定して失敗させる
	sources := []archive.Source{{Root: t.TempDir(), Path: "data/nonexistent"}}
	if err := archive.Create(context.Background(), archivePath, sources, nil); err == nil {
		t.Fatal("エラーになるはず")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("中途半端なファイルが残っている: %v", names)
	}
}

type zipEntry struct {
	name string
	body string
}

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()

	entries := make([]zipEntry, 0, len(files))
	for name, body := range files {
		entries = append(entries, zipEntry{name: name, body: body})
	}
	return writeZipOrdered(t, entries)
}

func writeZipOrdered(t *testing.T, entries []zipEntry) string {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		f, err := w.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// zip -r out.zip data で作ったアーカイブが丸ごと拒否されないこと。
func TestInspectAcceptsArchiveWithBareDataEntry(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: "data/"},
		{name: "data/world/"},
		{name: "data/world/level.dat", body: "nbt"},
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
	if m.LevelDir != "world" {
		t.Errorf("ワールド名を取れていない: %q", m.LevelDir)
	}
	// ディレクトリのエントリは数に入れない。
	if m.EntryCount != 1 {
		t.Errorf("エントリ数が %d（期待 1）", m.EntryCount)
	}
}

func TestExtractAcceptsArchiveWithBareDataEntry(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: "data/"},
		{name: "data/world/"},
		{name: "data/world/level.dat", body: "nbt"},
	})
	dest := t.TempDir()

	if err := archive.Extract(context.Background(), src, dest, archive.ExtractOptions{}, nil); err != nil {
		t.Fatalf("展開できなかった: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "data/world/level.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "nbt" {
		t.Errorf("中身が違う: %q", got)
	}
}
