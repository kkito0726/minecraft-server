package leveldat_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/leveldat"
)

func name(t *testing.T, s string) world.Name {
	t.Helper()

	n, err := world.NewName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// アダプタは読めなかった場合にエラーではなく「読めない」を返す。
// 破損したワールドが 1 つあっても一覧そのものが使えなくなってはいけない。
func TestAdapterReadWorld(t *testing.T) {
	t.Parallel()

	// testdata/ を data/ に見立てる。level.dat は testdata 直下ではなく
	// <名前>/level.dat に置く必要があるため、存在しない名前で試す。
	a := leveldat.NewAdapter("testdata")

	got := a.ReadWorld(context.Background(), name(t, "nonexistent"))
	if got.Readable() {
		t.Error("存在しないワールドが読めた扱いになっている")
	}
	if got.Display() != "不明" {
		t.Errorf("Display() が %q", got.Display())
	}
}

func TestAdapterRead(t *testing.T) {
	t.Parallel()

	a := leveldat.NewAdapter("testdata")

	t.Run("実データ", func(t *testing.T) {
		t.Parallel()
		got := a.Read(context.Background(), bytes.NewReader(realFixture(t)))
		if !got.Readable() {
			t.Fatal("読めていない")
		}
		dv, ok := got.DataVersion()
		if !ok || dv.Int32() != 4903 {
			t.Errorf("DataVersion が %v (ok=%v)", dv, ok)
		}
		if got.Name() != "26.2" {
			t.Errorf("Name が %q", got.Name())
		}
	})

	t.Run("壊れたデータ", func(t *testing.T) {
		t.Parallel()
		got := a.Read(context.Background(), strings.NewReader("これは NBT ではない"))
		if got.Readable() {
			t.Error("壊れたデータが読めた扱いになっている")
		}
	})
}

func TestAdapterReadFile(t *testing.T) {
	t.Parallel()

	a := leveldat.NewAdapter("testdata")

	version, lastPlayed, err := a.ReadFile("testdata/level.dat")
	if err != nil {
		t.Fatalf("ReadFile に失敗: %v", err)
	}
	if !version.Readable() {
		t.Error("読めていない")
	}
	if lastPlayed.IsZero() {
		t.Error("LastPlayed が取れていない")
	}

	if _, _, err := a.ReadFile("testdata/nonexistent.dat"); err == nil {
		t.Error("存在しないファイルはエラーになるはず")
	}
}

// DataVersion が 0 のワールドは「読めない」として扱う。
// バージョン比較ができないため、復元時に承諾を求める側へ倒す。
func TestAdapterTreatsZeroDataVersionAsUnreadable(t *testing.T) {
	t.Parallel()

	// Data に LevelName だけがあり DataVersion が無い NBT
	data := []byte{
		0x0a, 0x00, 0x00,
		0x0a, 0x00, 0x04, 'D', 'a', 't', 'a',
		0x08, 0x00, 0x09, 'L', 'e', 'v', 'e', 'l', 'N', 'a', 'm', 'e',
		0x00, 0x02, 'h', 'i',
		0x00,
		0x00,
	}

	a := leveldat.NewAdapter("testdata")
	got := a.Read(context.Background(), bytes.NewReader(data))

	if got.Readable() {
		t.Error("DataVersion が無いのに読めた扱いになっている")
	}
}
