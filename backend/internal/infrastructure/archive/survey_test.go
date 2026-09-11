package archive_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

/*
Survey は持ち込まれた zip の中身を見るためのもの。

Inspect と違って data/ 配下であることを要求しない。配布されている
ワールドは <名前>/level.dat の形をしているため、要求すると
「中身を見て判断する」こと自体ができなくなる。

展開先の外へ出る経路（.. / 絶対パス / シンボリックリンク）は
ここでも塞ぐ。緩めるのは置き場所の制限だけで、安全の制限ではない。
*/
func TestSurveyListsEntriesOutsideData(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"MyWorld/level.dat":        "nbt",
		"MyWorld/region/r.0.0.mca": "chunk",
	})

	got, err := archive.Survey(context.Background(), src)
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
	if len(got.LevelDatEntries) != 1 || got.LevelDatEntries[0] != "MyWorld/level.dat" {
		t.Errorf("level.dat を見つけられていない: %v", got.LevelDatEntries)
	}
	if got.TotalBytes != int64(len("nbt")+len("chunk")) {
		t.Errorf("合計サイズが %d", got.TotalBytes)
	}
}

func TestSurveyAcceptsArchiveUnderData(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"data/world/level.dat": "nbt"})

	got, err := archive.Survey(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.LevelDatEntries) != 1 || got.LevelDatEntries[0] != "data/world/level.dat" {
		t.Errorf("level.dat を見つけられていない: %v", got.LevelDatEntries)
	}
}

// 置き場所の制限を外しても、展開先の外へ出る経路は塞いだまま。
func TestSurveyStillRejectsEscapes(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"../etc/passwd", "/etc/passwd", `..\windows\system32`} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()

			src := writeZip(t, map[string]string{entry: "x"})
			if _, err := archive.Survey(context.Background(), src); !errors.Is(err, archive.ErrUnsafeEntry) {
				t.Errorf("%q を受理した: %v", entry, err)
			}
		})
	}
}

/*
Rewrap は持ち込まれたアーカイブを data/ 配下へ包み直す。

展開の側（allowedRoot）を緩めずに外部のワールドを受け入れるため、
入口で形を揃える。これで Extract の不変条件は 1 文字も変わらない。
*/
func TestRewrapMovesEntriesUnderData(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"MyWorld/level.dat":        "nbt",
		"MyWorld/region/r.0.0.mca": "chunk",
	})
	dest := filepath.Join(t.TempDir(), "wrapped.zip")

	if err := archive.Rewrap(context.Background(), src, dest, "data/"); err != nil {
		t.Fatalf("包み直せなかった: %v", err)
	}

	// 包み直した結果は、展開の検証を通るようになっていること。
	m, err := archive.Inspect(context.Background(), dest)
	if err != nil {
		t.Fatalf("包み直した結果が展開の検証を通らない: %v", err)
	}
	if m.LevelDir != "MyWorld" {
		t.Errorf("ワールド名が %q", m.LevelDir)
	}

	out := t.TempDir()
	if err := archive.Extract(context.Background(), dest, out, archive.ExtractOptions{}, nil); err != nil {
		t.Fatalf("展開できない: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(out, "data/MyWorld/region/r.0.0.mca"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "chunk" {
		t.Errorf("中身が違う: %q", body)
	}
}

// 包み直した結果が展開先の外を指してはいけない。
func TestRewrapRejectsEscapingEntries(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"../escape/level.dat": "nbt"})
	dest := filepath.Join(t.TempDir(), "wrapped.zip")

	if err := archive.Rewrap(context.Background(), src, dest, "data/"); !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("受理した: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("拒否したのに出力が残っている")
	}
}
