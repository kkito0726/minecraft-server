package archive_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

/*
Windows で作られた zip はエントリの区切りが \ になる。

archive は名前を / に正規化して受け入れる設計だが、「ディレクトリの
エントリか」の判定だけが生の名前を見ていた。そのため `data\world\` が
ファイルとして扱われ、0 バイトのファイルがディレクトリの位置に書かれる。

判定は 1 箇所（isDirEntry）に寄せてあるので、ここでは 4 つの入口
（Extract / Inspect / Survey / Rewrap）それぞれから確かめる。
*/

// 展開の不変条件「危険なエントリが 1 つでもあれば 1 バイトも書かない」は、
// ディレクトリをファイルと誤認した時点で破れる。0 バイトのファイルを
// data/world に書いたあと、その配下を作ろうとして失敗していた。
func TestExtractTreatsBackslashDirEntryAsDirectory(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: `data\world\`},
		{name: `data\world\region\`},
		{name: `data\world\region\r.0.0.mca`, body: "chunk"},
	})
	dest := t.TempDir()

	if err := archive.Extract(context.Background(), src, dest, archive.ExtractOptions{}, nil); err != nil {
		t.Fatalf("展開できなかった: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "data/world"))
	if err != nil {
		t.Fatalf("data/world が無い: %v", err)
	}
	if !info.IsDir() {
		t.Error("data/world がディレクトリになっていない")
	}
	got, err := os.ReadFile(filepath.Join(dest, "data/world/region/r.0.0.mca"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "chunk" {
		t.Errorf("中身が違う: %q", got)
	}
}

/*
RewritePrefix は data/world/ の形で渡ってくる（backupfs/store.go）。

生の名前のまま突き合わせると `data\world\level.dat` に一致せず、
「現在のワールド名として復元する」を選んだのに、アーカイブ自身の
名前のまま展開される。稼働中のワールドは変わらないので、利用者には
復元が効かなかったようにしか見えない。
*/
func TestExtractRewritesBackslashPrefix(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: `data\world\`},
		{name: `data\world\level.dat`, body: "level"},
	})
	dest := t.TempDir()

	opts := archive.ExtractOptions{
		RewritePrefix: map[string]string{"data/world/": "data/creative/"},
	}
	if err := archive.Extract(context.Background(), src, dest, opts, nil); err != nil {
		t.Fatalf("展開できなかった: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "data/creative/level.dat")); err != nil {
		t.Errorf("書き換え先に展開されていない: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "data/world/level.dat")); !os.IsNotExist(err) {
		t.Error("書き換え元にも展開されている")
	}
}

// EntryCount は「ファイルの数（ディレクトリを除く）」と定義してある。
func TestInspectExcludesBackslashDirEntries(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: `data\`},
		{name: `data\world\`},
		{name: `data\world\level.dat`, body: "nbt"},
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
	if m.EntryCount != 1 {
		t.Errorf("エントリ数が %d（期待 1）: %v", m.EntryCount, m.EntryNames)
	}
	if m.LevelDir != "world" {
		t.Errorf("ワールド名を取れていない: %q", m.LevelDir)
	}
}

func TestSurveyExcludesBackslashDirEntries(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: `MyWorld\datapacks\`},
		{name: `MyWorld\level.dat`, body: "nbt"},
	})

	got, err := archive.Survey(context.Background(), src)
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
	if got.EntryCount != 1 {
		t.Errorf("エントリ数が %d（期待 1）", got.EntryCount)
	}
}

/*
取り込みの経路がいちばん重い。壊れたアーカイブが黙って保管され、
壊れていると分かるのは後日それを復元しようとしたときになる。

ディレクトリのエントリは（/ 区切りの zip と同じく）写さない。
展開側が親ディレクトリを作るので、無くても中身は揃う。
*/
func TestRewrapDoesNotWriteBackslashDirEntriesAsFiles(t *testing.T) {
	t.Parallel()

	src := writeZipOrdered(t, []zipEntry{
		{name: `MyWorld\datapacks\`},
		{name: `MyWorld\level.dat`, body: "lvl"},
	})
	dest := filepath.Join(t.TempDir(), "out.zip")

	if err := archive.Rewrap(context.Background(), src, dest, "data/"); err != nil {
		t.Fatalf("包み直せなかった: %v", err)
	}

	r, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	if len(names) != 1 || names[0] != "data/MyWorld/level.dat" {
		t.Errorf("包み直しの中身が違う: %v", names)
	}
}
