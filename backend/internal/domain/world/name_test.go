package world_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// ワールド名はファイルシステムのパスになる。不正な名前を通すと
// data/ の外へ書き込めてしまうため、生成の時点で弾く。
func TestNewNameAccepts(t *testing.T) {
	t.Parallel()

	valid := []string{
		"world",
		"creative",
		"a",
		"My-World_2",
		"0",
		strings.Repeat("a", 32),
	}

	for _, s := range valid {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			n, err := world.NewName(s)
			if err != nil {
				t.Fatalf("受理されるはずが %v", err)
			}
			if n.String() != s {
				t.Errorf("String() が %q。%q のはず", n.String(), s)
			}
		})
	}
}

func TestNewNameRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"空", ""},
		{"33 文字", strings.Repeat("a", 33)},
		{"先頭がハイフン", "-world"},
		{"先頭がアンダースコア", "_world"},
		{"スラッシュを含む", "a/b"},
		{"親ディレクトリ", ".."},
		{"カレントディレクトリ", "."},
		{"相対パス", "../etc"},
		{"絶対パス", "/etc"},
		{"バックスラッシュ", `a\b`},
		{"空白を含む", "my world"},
		{"日本語", "ワールド"},
		{"ヌル文字", "a\x00b"},
		{"予約名 logs", "logs"},
		{"予約名 plugins", "plugins"},
		{"予約名 config", "config"},
		{"予約名 cache", "cache"},
		{"予約名 libraries", "libraries"},
		{"予約名 versions", "versions"},
		{"退避名（復元）", "world.broken-20260101-000000"},
		{"退避名（削除）", "world.deleted-20260101-000000"},
		{"ドットを含む", "world.backup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := world.NewName(tt.input); err == nil {
				t.Errorf("%q は拒否されるはず", tt.input)
			} else if !errors.Is(err, world.ErrInvalidName) {
				t.Errorf("ErrInvalidName を期待したが %v", err)
			}
		})
	}
}

// 予約名の判定は大文字小文字を区別しない。
// macOS のファイルシステムは既定で区別しないため、"LOGS" が logs と衝突する。
func TestNewNameRejectsReservedCaseInsensitively(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"LOGS", "Plugins", "CONFIG", "Cache"} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			if _, err := world.NewName(s); err == nil {
				t.Errorf("%q は拒否されるはず", s)
			}
		})
	}
}

// エラーメッセージは利用者にそのまま見せる。何が悪いか分かること。
func TestNewNameErrorMessageIsActionable(t *testing.T) {
	t.Parallel()

	_, err := world.NewName("my world")
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	msg := err.Error()
	if !strings.Contains(msg, "my world") {
		t.Errorf("エラーに入力値が含まれていない: %q", msg)
	}
}

// ゼロ値の Name は使えない。取り違えて空のパスを組み立てないため。
func TestZeroNameIsInvalid(t *testing.T) {
	t.Parallel()

	var n world.Name
	if n.IsValid() {
		t.Error("ゼロ値が有効になっている")
	}
	if n.String() != "" {
		t.Errorf("ゼロ値の String() が %q", n.String())
	}
}

// 同じ文字列から作った Name は等しい（値オブジェクト）。
func TestNameEquality(t *testing.T) {
	t.Parallel()

	a, err := world.NewName("world")
	if err != nil {
		t.Fatal(err)
	}
	b, err := world.NewName("world")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("同じ文字列から作った Name が等しくない")
	}
}

// 検証規則はフロントエンドと共有する。単一の出典から取れること。
func TestNamePatternIsExported(t *testing.T) {
	t.Parallel()

	if world.NamePattern == "" {
		t.Fatal("NamePattern が空。フロントエンドと規則を共有できない")
	}
	if world.MaxNameLength != 32 {
		t.Errorf("MaxNameLength が %d", world.MaxNameLength)
	}
	if len(world.ReservedNames()) == 0 {
		t.Error("ReservedNames が空")
	}
}
