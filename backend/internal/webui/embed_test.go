package webui

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// フロントエンドをビルドせずに mcadmind を起動すると白い画面になり原因が分かりにくい。
// IsBuilt はそれを起動時に検出するためのもので、この判定が壊れると
// フェーズ 19 の Pi へのデプロイで初めて気づくことになる。
func TestHasIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		assets fs.FS
		want   bool
	}{
		{
			name:   "index.html がある",
			assets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
			want:   true,
		},
		{
			name: "アセットはあるが index.html がない",
			assets: fstest.MapFS{
				"assets/index-abc.js":  {Data: []byte("//")},
				"assets/index-abc.css": {Data: []byte("/**/")},
			},
			want: false,
		},
		{
			name:   "空（.gitkeep だけの未ビルド状態）",
			assets: fstest.MapFS{".gitkeep": {Data: nil}},
			want:   false,
		},
		{
			name:   "index.html という名前のディレクトリは認めない",
			assets: fstest.MapFS{"index.html/dummy": {Data: []byte("x")}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasIndex(tt.assets); got != tt.want {
				t.Errorf("hasIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 読み取りに失敗するファイルツリーでは false を返す（パニックしない）。
func TestHasIndexReadError(t *testing.T) {
	t.Parallel()

	// ルートを読めない FS。fstest.MapFS を Sub で存在しないパスに向けて作る。
	broken, err := fs.Sub(fstest.MapFS{}, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if hasIndex(broken) {
		t.Error("読み取りに失敗したら false を返すはず")
	}
}

// Assets() は dist/ をルートとして返す。ルートがずれると配信パスが
// すべて /dist/ 配下になり、ブラウザから 404 になる。
func TestAssetsRootIsDist(t *testing.T) {
	t.Parallel()

	assets, err := Assets()
	if err != nil {
		t.Fatalf("Assets() が失敗した: %v", err)
	}

	// dist/ をルートにできているなら、"dist" というエントリは見えないはず
	if _, err := fs.Stat(assets, "dist"); err == nil {
		t.Error("Assets() のルートが dist/ の一段上になっている")
	}

	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		t.Fatalf("ルートを読めなかった: %v", err)
	}
	if len(entries) == 0 {
		t.Error("ルートが空。//go:embed all:dist が .gitkeep すら拾えていない")
	}
}

// IsBuilt は実際に埋め込まれた内容と整合していること。
func TestIsBuiltMatchesEmbeddedContent(t *testing.T) {
	t.Parallel()

	assets, err := Assets()
	if err != nil {
		t.Fatalf("Assets() が失敗した: %v", err)
	}
	_, statErr := fs.Stat(assets, indexFile)

	if got, want := IsBuilt(), statErr == nil; got != want {
		t.Errorf("IsBuilt() = %v だが、dist/%s の存在は %v", got, indexFile, want)
	}
}
