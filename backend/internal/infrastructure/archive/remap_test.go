package archive_test

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

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

// entryNames は出来上がったアーカイブのエントリ名を並べて返す。
func entryNames(t *testing.T, path string) []string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

// entryBody はエントリの中身を返す。
func entryBody(t *testing.T, path, entry string) string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rc.Close() }()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	t.Fatalf("%q が無い。あるのは %v", entry, entryNames(t, path))
	return ""
}

// stripData は data/ の皮を外す規則。試験用。
func stripData(entry string) string {
	rest, ok := strings.CutPrefix(entry, "data/")
	if !ok {
		return ""
	}
	return rest
}

// Remap はエントリ名を付け替えた新しいアーカイブを作る。
func TestRemapRenamesEntries(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/main/level.dat":        "nbt",
		"data/main/region/r.0.0.mca": "chunk",
	})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename: stripData,
	})
	if err != nil {
		t.Fatalf("包み直せなかった: %v", err)
	}

	want := []string{"main/level.dat", "main/region/r.0.0.mca"}
	if got := entryNames(t, dest); !slices.Equal(got, want) {
		t.Errorf("エントリが %v。%v のはず", got, want)
	}
}

// 中身は 1 バイトも変えない。変えるのは名前だけ。
func TestRemapPreservesContents(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/main/level.dat": "nbt"})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename: stripData,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := entryBody(t, dest, "main/level.dat"); got != "nbt" {
		t.Errorf("中身が %q。\"nbt\" のはず", got)
	}
}

// 空を返したエントリは持ち出さない。
func TestRemapDropsEntriesRenamedToEmpty(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/main/level.dat": "nbt",
		"outside/notes.txt":   "x",
	})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename: stripData,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := entryNames(t, dest); !slices.Equal(got, []string{"main/level.dat"}) {
		t.Errorf("エントリが %v。main/level.dat だけのはず", got)
	}
}

// Require で指定したエントリが無ければ、1 バイトも残さず失敗する。
// 中身の足りないアーカイブを渡すと、受け取った人は使うまで気づけない。
func TestRemapRejectsWhenRequiredEntryMissing(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/main/region/r.0.0.mca": "chunk"})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename:  stripData,
		Require: "main/level.dat",
	})
	if !errors.Is(err, archive.ErrRemapIncomplete) {
		t.Errorf("受理した: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("拒否したのに出力が残っている")
	}
}

// Require を満たしていれば通る。
func TestRemapAcceptsWhenRequiredEntryPresent(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/main/level.dat": "nbt"})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename:  stripData,
		Require: "main/level.dat",
	})
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
}

/*
安全の検証は Rename より先に通す。

置き場所の判断に委ねると、危険なエントリを「持ち出さない」で
黙って通してしまう。1 つでもあればアーカイブ全体を拒否する。
*/
func TestRemapRejectsUnsafeEntriesBeforeRenaming(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"../escape/level.dat", "/etc/passwd", `..\windows\system32`} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()

			src := writeZip(t, map[string]string{
				"data/main/level.dat": "nbt",
				entry:                 "x",
			})
			dest := filepath.Join(t.TempDir(), "out.zip")

			err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
				// すべて捨てる規則でも、危険なエントリは拒否に至る。
				Rename: func(string) string { return "" },
			})
			if !errors.Is(err, archive.ErrUnsafeEntry) {
				t.Errorf("受理した: %v", err)
			}
			if _, err := os.Stat(dest); err == nil {
				t.Error("拒否したのに出力が残っている")
			}
		})
	}
}

// ディレクトリのエントリは持ち出さない。展開のときに作られる。
func TestRemapSkipsDirectoryEntries(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/main/level.dat":  "nbt",
		"data/main/datapacks/": "",
	})
	dest := filepath.Join(t.TempDir(), "out.zip")

	err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{
		Rename: stripData,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := entryNames(t, dest); !slices.Equal(got, []string{"main/level.dat"}) {
		t.Errorf("エントリが %v。main/level.dat だけのはず", got)
	}
}

// 打ち切られたときに中途半端なアーカイブを残さない。
func TestRemapLeavesNothingOnCancel(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/main/level.dat": "nbt"})
	dest := filepath.Join(t.TempDir(), "out.zip")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := archive.Remap(ctx, src, dest, archive.RemapOptions{Rename: stripData}); !errors.Is(err, context.Canceled) {
		t.Errorf("打ち切りが返らない: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("打ち切ったのに出力が残っている")
	}
}

// 規則が無ければ何もしない。既定の挙動を決め打ちにすると、
// 渡し忘れたときに全部捨てたアーカイブが黙ってできる。
func TestRemapRequiresRenameRule(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/main/level.dat": "nbt"})
	dest := filepath.Join(t.TempDir(), "out.zip")

	if err := archive.Remap(context.Background(), src, dest, archive.RemapOptions{}); err == nil {
		t.Error("規則が無いのに通した")
	}
}
