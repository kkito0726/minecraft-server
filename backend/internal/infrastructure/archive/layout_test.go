package archive_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

/*
アーカイブには 2 つの形がある。

2026-10 より前に取ったものは data/<ワールド>/... の形。現在の保管形式は
<ワールド>/... で、シングルプレイの saves/ にそのまま置ける。

**復元は両方を受け付けなければならない。** 片方だけにすると、既に
保管してあるアーカイブが戻せなくなる。
*/

// --- 形の判定 ---

func TestInspectDetectsUnderDataLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/world/level.dat":        "nbt",
		"data/world/region/r.0.0.mca": "chunk",
		"data/plugins/x.jar":          "plugin",
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if m.Layout != archive.LayoutUnderData {
		t.Errorf("形が %v。LayoutUnderData のはず", m.Layout)
	}
	if m.LevelDir != "world" {
		t.Errorf("ワールド名が %q", m.LevelDir)
	}
}

func TestInspectDetectsWorldRootLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"world/level.dat":        "nbt",
		"world/region/r.0.0.mca": "chunk",
		"plugins/x.jar":          "plugin",
		"bukkit.yml":             "bukkit",
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if m.Layout != archive.LayoutWorldRoot {
		t.Errorf("形が %v。LayoutWorldRoot のはず", m.Layout)
	}
	if m.LevelDir != "world" {
		t.Errorf("ワールド名が %q", m.LevelDir)
	}
}

/*
data/ 配下と外が混ざったアーカイブは拒否する。

これは形の判定の都合ではなく**安全の要請**である。data/ の形と
判定したアーカイブは、展開の基点がプロジェクトディレクトリになる。
そこに data/ の外を指すエントリが混ざっていると、compose.yaml や
.env と並ぶ位置に書けてしまう。

逆に、ルートから始まる形だけのアーカイブは基点が data/ なので、
中身が何であれ data/ の外には出ない。
*/
func TestInspectRejectsMixedLayouts(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/world/level.dat": "nbt",
		"other/level.dat":      "nbt",
	})

	if _, err := archive.Inspect(context.Background(), src); !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("混ざったアーカイブを受理した: %v", err)
	}
}

/*
ルートから始まる形では、置き場所の制限を設けない。

<ワールド>/level.dat と etc/passwd は構造的に区別できない。ドメインの
知識（どれがワールド名でどれが対象のパスか）を archive に持ち込まずに
見分ける方法が無い。

安全は展開の基点が保証する。基点が data/ なので、どんな名前でも
data/ の外には出ない。何が入っているかは復元の事前確認が
ルートの集合として画面に出す。
*/
func TestExtractContainsArbitraryNamesWithinDataDir(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"etc/passwd": "x"})
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatalf("拒否された: %v", err)
	}
	if m.Layout != archive.LayoutWorldRoot {
		t.Fatalf("形が %v", m.Layout)
	}

	opts := archive.ExtractOptions{Layout: m.Layout}
	if err := archive.Extract(context.Background(), src, dataDir, opts, nil); err != nil {
		t.Fatal(err)
	}

	// data/ の中に収まっていること。外には出ていないこと。
	assertFile(t, filepath.Join(dataDir, "etc/passwd"), "x")
	if _, err := os.Stat(filepath.Join(root, "etc")); err == nil {
		t.Error("data/ の外に書かれている")
	}
}

// --- ルートの集合（復元の画面に出る）---

func TestInspectRootsUnderData(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/world/level.dat":         "nbt",
		"data/plugins/x.jar":           "plugin",
		"data/config/paper-global.yml": "config",
		"data/bukkit.yml":              "bukkit",
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"data/bukkit.yml", "data/config", "data/plugins", "data/world"}
	got := slices.Clone(m.Roots)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("ルートが %v。%v のはず", got, want)
	}
}

// 新しい形でもルートが出ること。空になると復元の画面が何も出さなくなる。
func TestInspectRootsWorldRoot(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"world/level.dat":         "nbt",
		"plugins/x.jar":           "plugin",
		"config/paper-global.yml": "config",
		"bukkit.yml":              "bukkit",
		"spigot.yml":              "spigot",
	})

	m, err := archive.Inspect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bukkit.yml", "config", "plugins", "spigot.yml", "world"}
	got := slices.Clone(m.Roots)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("ルートが %v。%v のはず", got, want)
	}
}

// --- 安全の検証は形に関わらず効く ---

func TestInspectRejectsEscapesInBothLayouts(t *testing.T) {
	t.Parallel()

	layouts := map[string]map[string]string{
		"data 配下": {"data/world/level.dat": "nbt"},
		"ルート":     {"world/level.dat": "nbt"},
	}

	for name, base := range layouts {
		for _, bad := range []string{"../escape/x", "/etc/passwd", `..\windows\system32`} {
			t.Run(name+"/"+bad, func(t *testing.T) {
				t.Parallel()

				files := map[string]string{}
				for k, v := range base {
					files[k] = v
				}
				files[bad] = "x"

				if _, err := archive.Inspect(context.Background(), writeZip(t, files)); !errors.Is(err, archive.ErrUnsafeEntry) {
					t.Errorf("%q を受理した: %v", bad, err)
				}
			})
		}
	}
}

// --- 展開 ---

/*
古い形は data/ を含んだ名前なので、プロジェクトディレクトリへ展開する。
新しい形はワールド名から始まるので、data/ へ展開する。

どちらもエントリ名を書き換えない。書き換えずに基点を変えるだけなので、
検証の不変条件が形ごとに増えない。
*/
func TestExtractPlacesUnderDataLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"data/world/level.dat": "nbt",
		"data/bukkit.yml":      "bukkit",
	})
	root := t.TempDir()

	opts := archive.ExtractOptions{Layout: archive.LayoutUnderData}
	if err := archive.Extract(context.Background(), src, root, opts, nil); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(root, "data/world/level.dat"), "nbt")
	assertFile(t, filepath.Join(root, "data/bukkit.yml"), "bukkit")
}

func TestExtractPlacesWorldRootLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{
		"world/level.dat": "nbt",
		"bukkit.yml":      "bukkit",
	})
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")

	opts := archive.ExtractOptions{Layout: archive.LayoutWorldRoot}
	if err := archive.Extract(context.Background(), src, dataDir, opts, nil); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(root, "data/world/level.dat"), "nbt")
	assertFile(t, filepath.Join(root, "data/bukkit.yml"), "bukkit")
}

// 復元先のワールド名の書き換えも、新しい形で効くこと。
func TestExtractRewritesLevelNameInWorldRootLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"world/level.dat": "nbt"})
	dataDir := filepath.Join(t.TempDir(), "data")

	opts := archive.ExtractOptions{
		Layout:        archive.LayoutWorldRoot,
		RewritePrefix: map[string]string{"world/": "creative/"},
	}
	if err := archive.Extract(context.Background(), src, dataDir, opts, nil); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(dataDir, "creative/level.dat"), "nbt")
}

// 新しい形でも展開先の外へは書かせない。
func TestExtractRejectsEscapeInWorldRootLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"world/level.dat": "nbt", "../escape": "x"})
	dataDir := filepath.Join(t.TempDir(), "data")

	opts := archive.ExtractOptions{Layout: archive.LayoutWorldRoot}
	if err := archive.Extract(context.Background(), src, dataDir, opts, nil); !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("受理した: %v", err)
	}
}

// 書き換えの結果が展開先の外を指すなら拒否する。
func TestExtractRejectsEscapingRewriteInWorldRootLayout(t *testing.T) {
	t.Parallel()

	src := writeZip(t, map[string]string{"world/level.dat": "nbt"})
	dataDir := filepath.Join(t.TempDir(), "data")

	opts := archive.ExtractOptions{
		Layout:        archive.LayoutWorldRoot,
		RewritePrefix: map[string]string{"world/": "../escape/"},
	}
	if err := archive.Extract(context.Background(), src, dataDir, opts, nil); !errors.Is(err, archive.ErrUnsafeEntry) {
		t.Errorf("受理した: %v", err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s を読めない: %v", path, err)
	}
	if string(body) != want {
		t.Errorf("%s の中身が %q。%q のはず", path, body, want)
	}
}
