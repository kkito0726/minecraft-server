package dotenv_test

import (
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/config/dotenv"
)

func parse(t *testing.T, s string) *dotenv.File {
	t.Helper()

	f, err := dotenv.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}
	return f
}

func TestParseValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		key   string
		want  string
	}{
		{"素の値", "K=v", "K", "v"},
		{"空の値", "K=", "K", ""},
		{"ダブルクォート", `K="a b"`, "K", "a b"},
		{"シングルクォート", `K='a b'`, "K", "a b"},
		{"値に = を含む", "K=a=b=c", "K", "a=b=c"},
		{"行内コメント", "K=v  # 説明", "K", "v"},
		{"引用符の中の #", `K="a # b"`, "K", "a # b"},
		{"引用符の中の空白は保持", `K="  spaced  "`, "K", "  spaced  "},
		{"エスケープしたダブルクォート", `K="say \"hi\""`, "K", `say "hi"`},
		{"エスケープしたドル記号", `K="\$HOME"`, "K", "$HOME"},
		{"シングルクォート内はエスケープしない", `K='a\nb'`, "K", `a\nb`},
		{"値の前後の空白は落とす", "K=  v  ", "K", "v"},
		{"日本語の値", "K=こんにちは", "K", "こんにちは"},
		{"閉じていない引用符はそのまま", `K="unclosed`, "K", `"unclosed`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parse(t, tt.input).Get(tt.key)
			if !ok {
				t.Fatalf("キー %s が見つからない", tt.key)
			}
			if got != tt.want {
				t.Errorf("値が %q。%q のはず", got, tt.want)
			}
		})
	}
}

func TestParseNonAssignmentLines(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"# コメント",
		"",
		"   ",
		"これは代入ではない",
		"=値だけ",
		"9INVALID=x", // 数字で始まるキーは無効
		"VALID=x",
	}, "\n")

	f := parse(t, input)
	if got := f.Keys(); len(got) != 1 || got[0] != "VALID" {
		t.Errorf("キーが %v。VALID だけのはず", got)
	}

	// 代入として解釈されなかった行も、そのまま保持されること
	out := render(t, f)
	for _, line := range []string{"# コメント", "これは代入ではない", "=値だけ", "9INVALID=x"} {
		if !strings.Contains(out, line) {
			t.Errorf("行が失われている: %q", line)
		}
	}
}

func TestGetMissingKey(t *testing.T) {
	t.Parallel()

	if _, ok := parse(t, "K=v").Get("NOPE"); ok {
		t.Error("存在しないキーで ok が真になった")
	}
}

func TestKeysPreservesOrder(t *testing.T) {
	t.Parallel()

	f := parse(t, "C=3\n# コメント\nA=1\n\nB=2")
	want := []string{"C", "A", "B"}
	got := f.Keys()

	if len(got) != len(want) {
		t.Fatalf("キーが %v。%v のはず", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d 番目が %q。%q のはず", i, got[i], want[i])
		}
	}
}

func TestWithout(t *testing.T) {
	t.Parallel()

	f := parse(t, "# A の説明\nA=1\nB=2")
	after := f.Without("A")

	if _, ok := after.Get("A"); ok {
		t.Error("A が残っている")
	}
	if _, ok := after.Get("B"); !ok {
		t.Error("B まで消えている")
	}
	// 元は変わらない（不変）
	if _, ok := f.Get("A"); !ok {
		t.Error("元の File が変更された")
	}
	// コメントは判別できないので残す
	if !strings.Contains(render(t, after), "# A の説明") {
		t.Error("コメントまで消している")
	}
}

func TestWithIsImmutable(t *testing.T) {
	t.Parallel()

	f := parse(t, "K=original")
	updated := f.With("K", "changed")

	if got, _ := f.Get("K"); got != "original" {
		t.Errorf("元の File が変更された: %q", got)
	}
	if got, _ := updated.Get("K"); got != "changed" {
		t.Errorf("新しい File に反映されていない: %q", got)
	}
}

// 書き込む値に応じて引用符を付ける。付けないと source が壊れる。
func TestWithQuotesWhenNeeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"安全な値は素のまま", "simple-value_1.2:3/4", "K=simple-value_1.2:3/4"},
		{"空白を含む", "a b", `K="a b"`},
		{"ダブルクォートを含む", `a"b`, `K="a\"b"`},
		{"ドル記号を含む", "$HOME", `K="\$HOME"`},
		{"バッククォートを含む", "`cmd`", "K=\"\\`cmd\\`\""},
		{"バックスラッシュを含む", `a\b`, `K="a\\b"`},
		{"日本語", "こんにちは", `K="こんにちは"`},
		{"空文字は括らない", "", "K="},
		{"# を含む", "a#b", `K="a#b"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.TrimRight(render(t, parse(t, "K=x").With("K", tt.value)), "\n")
			if got != tt.want {
				t.Errorf("出力が %q。%q のはず", got, tt.want)
			}
		})
	}
}

// 元が引用符つきなら、値が安全でも引用符を維持する。
// MC_MOTD のように「引用してあること自体が意図」の行を壊さないため。
func TestWithKeepsExistingQuoteStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		value string
		want  string
	}{
		{"ダブルクォートを維持", `K="old value"`, "safe", `K="safe"`},
		{"シングルクォートを維持", `K='old value'`, "safe", `K='safe'`},
		{"引用符なしは値次第", "K=old", "safe", "K=safe"},
		{"シングルクォート内に ' が来たらダブルに切り替える", `K='old'`, "it's", `K="it's"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.TrimRight(render(t, parse(t, tt.input).With("K", tt.value)), "\n")
			if got != tt.want {
				t.Errorf("出力が %q。%q のはず", got, tt.want)
			}
		})
	}
}

// 行内コメントは値を変えても残る。
func TestWithKeepsTrailingComment(t *testing.T) {
	t.Parallel()

	got := strings.TrimRight(render(t, parse(t, "K=old  # 説明").With("K", "new")), "\n")
	if got != "K=new  # 説明" {
		t.Errorf("出力が %q", got)
	}
}

// 空のファイルでも壊れない。
func TestParseEmpty(t *testing.T) {
	t.Parallel()

	f := parse(t, "")
	if len(f.Keys()) != 0 {
		t.Error("キーがあるはずがない")
	}
	if got := render(t, f); got != "" {
		t.Errorf("出力が %q。空のはず", got)
	}
	// 空のファイルへの追加も動くこと
	if got, _ := f.With("K", "v").Get("K"); got != "v" {
		t.Error("空のファイルに追加できない")
	}
}

// 末尾の改行の有無を保つ。
func TestTrailingNewlinePreserved(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"K=v\n", "K=v"} {
		got := render(t, parse(t, input))
		// Parse は行単位で読むため、末尾の改行は正規化されて 1 つになる
		if !strings.HasSuffix(got, "K=v\n") {
			t.Errorf("入力 %q の出力が %q", input, got)
		}
	}
}
