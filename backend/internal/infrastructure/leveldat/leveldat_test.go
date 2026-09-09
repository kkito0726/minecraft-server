package leveldat_test

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/leveldat"
)

func realFixture(t *testing.T) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", "level.dat"))
	if err != nil {
		t.Fatalf("フィクスチャを読めない: %v", err)
	}
	return b
}

// 実際の Paper 26.2 が書いた level.dat を読めること。
// 合成したデータだけで検証すると、実装が実物とずれていても気づけない。
func TestReadRealLevelDat(t *testing.T) {
	t.Parallel()

	info, err := leveldat.Read(bytes.NewReader(realFixture(t)))
	if err != nil {
		t.Fatalf("Read に失敗: %v", err)
	}

	if got := info.LevelName; got != "world" {
		t.Errorf("LevelName が %q。world のはず", got)
	}
	if got := info.VersionName; got != "26.2" {
		t.Errorf("VersionName が %q。26.2 のはず", got)
	}
	// バージョン判定のキー。表示文字列ではなくこの整数で比較する。
	if got := info.DataVersion; got != 4903 {
		t.Errorf("DataVersion が %d。4903 のはず", got)
	}
	if info.Snapshot {
		t.Error("Snapshot が真になっている。正式リリース版のはず")
	}
	if info.LastPlayed.IsZero() {
		t.Error("LastPlayed が設定されていない")
	}
	// 未来の日付や 1970 年になっていないこと（ミリ秒/秒の取り違え検出）
	if info.LastPlayed.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("LastPlayed が %v。ミリ秒を秒として解釈していないか", info.LastPlayed)
	}
	if !strings.Contains(info.ServerBrand, "Paper") {
		t.Errorf("ServerBrand が %q。Paper を含むはず", info.ServerBrand)
	}
}

// level.dat は gzip で保存されるが、非圧縮や zlib のこともある。
// 先頭のマジックバイトで判別する。
func TestReadCompressionFormats(t *testing.T) {
	t.Parallel()

	raw := gunzip(t, realFixture(t))

	tests := []struct {
		name string
		data []byte
	}{
		{"gzip", realFixture(t)},
		{"非圧縮", raw},
		{"zlib", zlibCompress(t, raw)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, err := leveldat.Read(bytes.NewReader(tt.data))
			if err != nil {
				t.Fatalf("Read に失敗: %v", err)
			}
			if info.DataVersion != 4903 {
				t.Errorf("DataVersion が %d", info.DataVersion)
			}
		})
	}
}

// 壊れた入力でパニックせずエラーを返すこと。
// 破損したワールドや将来の形式変更でここが落ちると、一覧表示ごと使えなくなる。
func TestReadMalformed(t *testing.T) {
	t.Parallel()

	real := realFixture(t)

	tests := []struct {
		name string
		data []byte
	}{
		{"空", nil},
		{"gzip の途中で切れている", real[:len(real)/2]},
		{"gzip ヘッダだけ", real[:10]},
		{"不正なマジックバイト", []byte{0xff, 0xfe, 0xfd, 0xfc}},
		{"NBT の途中で切れている", gunzip(t, real)[:100]},
		{"ルートが TAG_Compound でない", []byte{0x08, 0x00, 0x00}},
		{"文字列長が実データを超える", []byte{0x0a, 0x00, 0x00, 0x08, 0x7f, 0xff, 0x41}},
		{"配列長が巨大", []byte{0x0a, 0x00, 0x00, 0x07, 0x00, 0x01, 0x41, 0x7f, 0xff, 0xff, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := leveldat.Read(bytes.NewReader(tt.data)); err == nil {
				t.Error("エラーになるはず")
			}
		})
	}
}

// Data が無い NBT は、形式としては正しいが level.dat ではない。
func TestReadMissingDataCompound(t *testing.T) {
	t.Parallel()

	// ルート TAG_Compound の中に "Other" という TAG_Compound だけがある
	data := []byte{
		0x0a, 0x00, 0x00, // ルート TAG_Compound（名前なし）
		0x0a, 0x00, 0x05, 'O', 't', 'h', 'e', 'r', // TAG_Compound "Other"
		0x00, // Other の TAG_End
		0x00, // ルートの TAG_End
	}

	_, err := leveldat.Read(bytes.NewReader(data))
	if !errors.Is(err, leveldat.ErrNotLevelDat) {
		t.Errorf("ErrNotLevelDat を期待したが %v", err)
	}
}

// 読めないフィールドがあっても、読めたものは返す。
// level.dat の構造はバージョンで変わるため、全部そろっていることを前提にしない。
func TestReadPartialFields(t *testing.T) {
	t.Parallel()

	// Data の中に LevelName と DataVersion だけがある
	data := []byte{
		0x0a, 0x00, 0x00,
		0x0a, 0x00, 0x04, 'D', 'a', 't', 'a',
		0x08, 0x00, 0x09, 'L', 'e', 'v', 'e', 'l', 'N', 'a', 'm', 'e',
		0x00, 0x02, 'h', 'i',
		0x03, 0x00, 0x0b, 'D', 'a', 't', 'a', 'V', 'e', 'r', 's', 'i', 'o', 'n',
		0x00, 0x00, 0x13, 0x27, // 4903
		0x00,
		0x00,
	}

	info, err := leveldat.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read に失敗: %v", err)
	}
	if info.LevelName != "hi" {
		t.Errorf("LevelName が %q", info.LevelName)
	}
	if info.DataVersion != 4903 {
		t.Errorf("DataVersion が %d", info.DataVersion)
	}
	// 無かったフィールドはゼロ値
	if info.VersionName != "" {
		t.Errorf("VersionName が %q。空のはず", info.VersionName)
	}
	if !info.LastPlayed.IsZero() {
		t.Error("LastPlayed がゼロ値でない")
	}
}

// ReadFile は存在しないパスで明確なエラーを返す。
func TestReadFileMissing(t *testing.T) {
	t.Parallel()

	if _, err := leveldat.ReadFile(filepath.Join(t.TempDir(), "level.dat")); err == nil {
		t.Error("エラーになるはず")
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	info, err := leveldat.ReadFile(filepath.Join("testdata", "level.dat"))
	if err != nil {
		t.Fatalf("ReadFile に失敗: %v", err)
	}
	if info.DataVersion != 4903 {
		t.Errorf("DataVersion が %d", info.DataVersion)
	}
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()

	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zlibCompress(t *testing.T, b []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
