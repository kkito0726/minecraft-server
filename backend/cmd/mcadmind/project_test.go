package main

import (
	"os"
	"path/filepath"
	"testing"
)

/*
compose のプロジェクト名は、指し示されたディレクトリから決まらなければならない。

固定の定数にしていると、-project-dir を別のディレクトリに向けても
**同じプロジェクト**を操作することになる。テスト用の環境に向けたつもりの
mcadmind が本番のコンテナを down させる。実際にそれで本番が止まった。

compose.yaml の name: を読むのは、その値こそが「このディレクトリの
プロジェクト名」だから。人が別のキーを設定し忘れても正しく動く。
*/
func TestComposeProjectNameFromFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		compose string
		want    string
	}{
		{
			name:    "name: を読む",
			compose: "name: minecraft-server-test\nservices:\n  mc:\n    image: x\n",
			want:    "minecraft-server-test",
		},
		{
			name:    "引用符つきでも読む",
			compose: "name: \"my-project\"\nservices: {}\n",
			want:    "my-project",
		},
		{
			name:    "コメントと空行があっても読む",
			compose: "# 説明\n\nname: from-comment\nservices: {}\n",
			want:    "from-comment",
		},
		{
			name:    "行末のコメントは値に含めない",
			compose: "name: trailing # 説明\nservices: {}\n",
			want:    "trailing",
		},
		// 入れ子の name: は別物。services の中の設定を拾ってはいけない。
		{
			name:    "字下げされた name: は無視する",
			compose: "services:\n  mc:\n    name: inner\n",
			want:    defaultComposeProject,
		},
		{
			name:    "name: が無ければ既定",
			compose: "services:\n  mc:\n    image: x\n",
			want:    defaultComposeProject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(dir, "compose.yaml"), []byte(tt.compose), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := composeProjectFrom(dir, ""); got != tt.want {
				t.Errorf("プロジェクト名が %q（期待 %q）", got, tt.want)
			}
		})
	}
}

// 明示された値が最優先。compose.yaml を書き換えられない場合の逃げ道。
func TestComposeProjectNameFromConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"),
		[]byte("name: from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := composeProjectFrom(dir, "from-env"); got != "from-env" {
		t.Errorf("設定より compose.yaml を優先している: %q", got)
	}
}

// compose.yaml が読めなくても起動は止めない。
// 既定に倒せば、これまでと同じ振る舞いになる。
func TestComposeProjectNameWithoutFile(t *testing.T) {
	t.Parallel()

	if got := composeProjectFrom(t.TempDir(), ""); got != defaultComposeProject {
		t.Errorf("既定に倒れていない: %q", got)
	}
}
