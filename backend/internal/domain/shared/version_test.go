package shared_test

import (
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// DataVersion は level.dat の Data.DataVersion。バージョン比較のキー。
// 表示文字列と混同しないよう専用の型にしてある。
func TestNewDataVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   int32
		wantErr bool
	}{
		{"実測値", 4903, false},
		{"最小", 1, false},
		{"ゼロは不正", 0, true},
		{"負は不正", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, err := shared.NewDataVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("エラーになるはず")
				}
				return
			}
			if err != nil {
				t.Fatalf("受理されるはずが %v", err)
			}
			if v.Int32() != tt.input {
				t.Errorf("Int32() が %d", v.Int32())
			}
		})
	}
}

func TestDataVersionComparison(t *testing.T) {
	t.Parallel()

	older := mustVersion(t, 4820)
	newer := mustVersion(t, 4903)

	if !older.LessThan(newer) {
		t.Error("4820 < 4903 のはず")
	}
	if newer.LessThan(older) {
		t.Error("4903 < 4820 になっている")
	}
	if !older.Equals(mustVersion(t, 4820)) {
		t.Error("同じ値が等しくない")
	}
	if older.Equals(newer) {
		t.Error("違う値が等しくなっている")
	}
}

// WorldVersion は level.dat から読んだ内容。読めなかった場合を表現できること。
// 「読めなかった」と「バージョン 0」を区別できないと、
// 破損したワールドを最古のバージョンとして扱ってしまう。
func TestWorldVersion(t *testing.T) {
	t.Parallel()

	t.Run("読めた場合", func(t *testing.T) {
		t.Parallel()
		v := shared.NewWorldVersion("26.2", mustVersion(t, 4903), false, "world")

		if !v.Readable() {
			t.Error("Readable が偽")
		}
		if v.Name() != "26.2" {
			t.Errorf("Name() が %q", v.Name())
		}
		if got, ok := v.DataVersion(); !ok || got.Int32() != 4903 {
			t.Errorf("DataVersion() が %v, %v", got, ok)
		}
		if v.LevelName() != "world" {
			t.Errorf("LevelName() が %q", v.LevelName())
		}
	})

	t.Run("読めなかった場合", func(t *testing.T) {
		t.Parallel()
		v := shared.UnreadableWorldVersion()

		if v.Readable() {
			t.Error("Readable が真")
		}
		if _, ok := v.DataVersion(); ok {
			t.Error("読めていないのに DataVersion が取れる")
		}
		if got := v.Display(); got != "不明" {
			t.Errorf("Display() が %q。不明 のはず", got)
		}
	})

	t.Run("スナップショットは表示に併記する", func(t *testing.T) {
		t.Parallel()
		v := shared.NewWorldVersion("26.3-pre1", mustVersion(t, 4910), true, "world")
		if got := v.Display(); got != "26.3-pre1（スナップショット）" {
			t.Errorf("Display() が %q", got)
		}
	})
}

// ゼロ値の WorldVersion は「読めなかった」として扱う。
// 取り違えて有効なバージョンとして比較に使わないため。
func TestZeroWorldVersionIsUnreadable(t *testing.T) {
	t.Parallel()

	var v shared.WorldVersion
	if v.Readable() {
		t.Error("ゼロ値が Readable になっている")
	}
	if _, ok := v.DataVersion(); ok {
		t.Error("ゼロ値から DataVersion が取れる")
	}
}

func mustVersion(t *testing.T, v int32) shared.DataVersion {
	t.Helper()

	dv, err := shared.NewDataVersion(v)
	if err != nil {
		t.Fatal(err)
	}
	return dv
}
