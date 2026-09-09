package dotenv_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/dotenv"
)

// .env には日本語のコメントが 37 行以上あり、値の一部は source 互換のために
// 引用符で括られている。マップにシリアライズし直すとこれらが全部消える。
//
// 「1 行だけ変わる」ことを差分で確かめるのが、コメント保持の唯一の実効的な保証。
// 個別のフィールドを検査する形だと、たとえば行の順序が入れ替わっても気づけない。
func TestUpdateChangesExactlyOneLine(t *testing.T) {
	t.Parallel()

	original := loadFixture(t)

	f, err := dotenv.Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}

	updated := render(t, f.With("MC_LEVEL", "creative"))

	added, removed := diffLines(string(original), updated)
	if len(added) != 1 || len(removed) != 1 {
		t.Fatalf("変更は 1 行のみのはず。追加 %d 行 / 削除 %d 行\n追加: %q\n削除: %q",
			len(added), len(removed), added, removed)
	}
	if removed[0] != "MC_LEVEL=world" {
		t.Errorf("削除された行が %q。MC_LEVEL の行のはず", removed[0])
	}
	if added[0] != "MC_LEVEL=creative" {
		t.Errorf("追加された行が %q", added[0])
	}
}

// 新しいキーは末尾に追加される。既存の行は 1 行も動かない。
func TestAddOnlyAppends(t *testing.T) {
	t.Parallel()

	original := loadFixture(t)

	f, err := dotenv.Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}

	updated := render(t, f.With("BRAND_NEW_KEY", "value"))

	added, removed := diffLines(string(original), updated)
	if len(removed) != 0 {
		t.Errorf("既存の行が消えている: %q", removed)
	}
	// 空行 + コメント + 代入 の 3 行以内に収まること
	if len(added) == 0 || len(added) > 3 {
		t.Fatalf("追加は 1〜3 行のはず。実際 %d 行: %q", len(added), added)
	}
	if !strings.Contains(updated, "BRAND_NEW_KEY=value") {
		t.Error("追加したキーが出力に無い")
	}
	// 元の内容が完全に前方一致で残っていること
	if !strings.HasPrefix(updated, strings.TrimRight(string(original), "\n")) {
		t.Error("既存の内容が先頭から変わっている。末尾追加になっていない")
	}
}

// 値を変えない With は、バイト単位で元と同一の出力を返す。
// 「読んで書き戻しただけで差分が出る」状態だと、楽観ロックが常に衝突する。
func TestRoundTripIsByteIdentical(t *testing.T) {
	t.Parallel()

	original := loadFixture(t)

	f, err := dotenv.Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}

	if got := render(t, f); got != string(original) {
		added, removed := diffLines(string(original), got)
		t.Errorf("読んで書き戻しただけで差分が出た\n追加: %q\n削除: %q", added, removed)
	}
}

// 引用符つきの値を持つ行を、別のキーの更新で壊さない。
// docs/backup-restore.md が「クォートを外すと source が失敗する」と明記している。
func TestQuotingPreservedAcrossUpdates(t *testing.T) {
	t.Parallel()

	original := loadFixture(t)

	f, err := dotenv.Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}

	updated := render(t, f.With("MC_LEVEL", "creative"))

	for _, line := range []string{
		`MC_MOTD="A Minecraft Server on Docker"`,
		`QUOTED_WITH_SPACE="値に 空白 を含む"`,
		`SINGLE_QUOTED='シングルクォート'`,
		`INLINE_COMMENT=value  # これは行内コメント`,
	} {
		if !strings.Contains(updated, line) {
			t.Errorf("行が保持されていない: %s", line)
		}
	}
}

// フィクスチャは実際の .env.example から作ってあるので、
// 書き戻したものが引き続き source できることを確かめる。
func TestOutputStaysSourceable(t *testing.T) {
	t.Parallel()

	original := loadFixture(t)
	f, err := dotenv.Parse(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Parse に失敗: %v", err)
	}

	// 空白・記号・引用符を含む値を書き込んでも壊れないこと
	updated := f.
		With("MC_MOTD", `新しい "MOTD" と $変数 と \バックスラッシュ`).
		With("MC_LEVEL", "creative")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(render(t, updated)), 0o600); err != nil {
		t.Fatal(err)
	}

	got := sourceAndEcho(t, path, "MC_MOTD")
	want := `新しい "MOTD" と $変数 と \バックスラッシュ`
	if got != want {
		t.Errorf("source した結果が %q。%q のはず", got, want)
	}
	if lvl := sourceAndEcho(t, path, "MC_LEVEL"); lvl != "creative" {
		t.Errorf("MC_LEVEL が %q", lvl)
	}
}
