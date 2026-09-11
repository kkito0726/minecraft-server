package archive

import (
	"errors"
	"io/fs"
	"testing"
)

// 展開先の外を指すエントリを 1 つも通さないこと。
// ここが緩むと、アーカイブの中身次第でリポジトリの外にファイルを書ける。
func TestSafeEntryName(t *testing.T) {
	t.Parallel()

	valid := []struct {
		input string
		want  string
	}{
		{"data/world/level.dat", "data/world/level.dat"},
		{"data/spigot.yml", "data/spigot.yml"},
		{"data/world/dimensions/minecraft/overworld/region/r.0.0.mca",
			"data/world/dimensions/minecraft/overworld/region/r.0.0.mca"},
		// 冗長な表記は正規化して受理する
		{"data/./world/level.dat", "data/world/level.dat"},
		{"data/world/../world/level.dat", "data/world/level.dat"},
		{`data\world\level.dat`, "data/world/level.dat"},
	}

	for _, tt := range valid {
		t.Run("受理/"+tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := safeEntryName(tt.input)
			if err != nil {
				t.Fatalf("受理されるはずが %v", err)
			}
			if got != tt.want {
				t.Errorf("正規化結果が %q。%q のはず", got, tt.want)
			}
		})
	}

	invalid := []string{
		"",
		"../etc/passwd",
		"../../etc/passwd",
		"/etc/passwd",
		"/data/world/level.dat",
		"etc/passwd",
		"data/../../tmp/x",
		"data/world/../../../tmp/x",
		`C:\Windows\system32`,
		`data\..\..\tmp\x`,
		"..",
		"world/level.dat",
	}

	for _, input := range invalid {
		t.Run("拒否/"+input, func(t *testing.T) {
			t.Parallel()
			if _, err := safeEntryName(input); !errors.Is(err, ErrUnsafeEntry) {
				t.Errorf("%q は拒否されるはず。err=%v", input, err)
			}
		})
	}
}

func TestCheckMode(t *testing.T) {
	t.Parallel()

	allowed := []fs.FileMode{0o644, 0o755, fs.ModeDir | 0o755}
	for _, mode := range allowed {
		if err := checkMode("data/x", mode); err != nil {
			t.Errorf("%v は許可されるはずが %v", mode, err)
		}
	}

	denied := []fs.FileMode{
		fs.ModeSymlink | 0o777,
		fs.ModeDevice | 0o644,
		fs.ModeNamedPipe | 0o644,
		fs.ModeSocket | 0o644,
	}
	for _, mode := range denied {
		if err := checkMode("data/x", mode); !errors.Is(err, ErrUnsafeEntry) {
			t.Errorf("%v は拒否されるはずが %v", mode, err)
		}
	}
}

/*
zip -r out.zip data のように data ディレクトリごと固めると、
アーカイブに "data/" というエントリが入る。

path.Clean は末尾の / を落とすので "data/" は "data" になり、
"data/" 配下かどうかの判定に落ちてしまう。1 つでも違反があれば
アーカイブ全体を拒否する作りなので、**zip 全体が復元できなくなる**。

手で作った zip を持ち込んだときにだけ起きる。管理コンソール自身が
作るアーカイブにはこのエントリが無いため、気づく機会が無かった。
*/
func TestSafeEntryNameAcceptsBareDataDirectory(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"data/", `data\`, "./data/"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got, err := safeEntryName(input)
			if err != nil {
				t.Fatalf("%q を拒否した: %v", input, err)
			}
			if got != "data" {
				t.Errorf("正規化の結果が %q（期待 %q）", got, "data")
			}
		})
	}
}

// ディレクトリとして書かれていない "data" は data/ の外にある
// ただのファイルなので、通してはいけない。
func TestSafeEntryNameRejectsFileNamedData(t *testing.T) {
	t.Parallel()

	if _, err := safeEntryName("data"); !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("data という名前のファイルを受理した: %v", err)
	}
}

// 緩めた結果、別のディレクトリまで通していないこと。
func TestSafeEntryNameStillRejectsOtherRoots(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"MyWorld/", "plugins/", "/", "../data/"} {
		if _, err := safeEntryName(input); !errors.Is(err, ErrUnsafeEntry) {
			t.Errorf("%q を受理した: %v", input, err)
		}
	}
}
