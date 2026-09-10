package http

import (
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// spaHandler は静的ファイルを配信し、未知のパスは index.html に落とす。
//
// クライアント側ルーティング（/worlds など）を直接開いても 404 に
// ならないようにするため。
func spaHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(assets, path); err != nil {
			// 未知のパス。SPA のルーティングに任せる。
			serveIndex(w, r, assets)
			return
		}

		setCacheHeaders(w, path)
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	body, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "フロントエンドがビルドされていません", http.StatusNotFound)
		return
	}
	// index.html は毎回取り直す。古いものが残ると、新しいアセットを
	// 指せずに白い画面になる。
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", zeroTime, strings.NewReader(string(body)))
}

// setCacheHeaders はハッシュ付きのアセットだけ長期キャッシュする。
func setCacheHeaders(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "assets/") {
		// Vite が内容のハッシュをファイル名に埋めるので、
		// 中身が変われば名前も変わる。長期キャッシュして安全。
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
}

// securityHeaders はブラウザ側の防御を有効にする。
//
// トークンを localStorage に持つ設計のため、XSS の影響を抑える意味が大きい。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// 外部からのスクリプト読み込みと外部への通信を禁じる。
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self'",
			"style-src 'self' 'unsafe-inline'",
			"img-src 'self' data:",
			"connect-src 'self'",
			"frame-ancestors 'none'",
			"base-uri 'self'",
			"form-action 'self'",
		}, "; "))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// zeroTime は ServeContent に渡す修正時刻。
// 埋め込みファイルには意味のある時刻が無いので、条件付き GET を無効にする。
var zeroTime = time.Time{}
