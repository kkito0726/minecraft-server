package archive_test

import (
	"context"
	"errors"
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
