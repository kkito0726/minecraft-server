// Package webui は Vite でビルドしたフロントエンドをバイナリに埋め込んで配信する。
//
// Vite の build.outDir をこのパッケージの dist/ に向けているのは、Go の //go:embed が
// 親ディレクトリを参照できないため。dist/ の中身は生成物なので Git では追跡せず、
// .gitkeep だけを置いてある（embed は存在しないディレクトリを指すとコンパイルに失敗する）。
//
// 注意: embed したデータは、そのパッケージが実際に参照されないとリンカに削除される。
// main が webui を import していないとバイナリにアセットが入らず、Pi へ配置して
// 初めて白い画面で気づくことになる。
package webui

import (
	"embed"
	"io/fs"
)

// all: を付けると _ や . で始まるファイルも含まれる。Vite が出す .vite/ などを
// 取りこぼさないため、および .gitkeep だけの状態でもコンパイルを通すために必要。
//
//go:embed all:dist
var distFS embed.FS

// indexFile は配信の起点。これが無ければフロントエンドは未ビルドとみなす。
const indexFile = "index.html"

// Assets は配信するファイルツリーを dist/ をルートとして返す。
func Assets() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// IsBuilt はフロントエンドがビルド済みかを返す。
// 開発中は Vite の dev サーバーを使うため dist/ が空のことがあり、その場合に
// 「白い画面」ではなく起動時にビルド手順を案内するために使う。
func IsBuilt() bool {
	assets, err := Assets()
	if err != nil {
		return false
	}
	return hasIndex(assets)
}

// hasIndex は Assets() の中身から判定を切り離したもの。
// embed 済みの実体では到達できない分岐（読み取り失敗・index.html 不在）を
// テストできるように、ファイルツリーを引数で受け取る。
func hasIndex(assets fs.FS) bool {
	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && e.Name() == indexFile {
			return true
		}
	}
	return false
}
