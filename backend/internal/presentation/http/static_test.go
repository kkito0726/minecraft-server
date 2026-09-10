package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func assets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc123.css": {Data: []byte("body{}")},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

// クライアント側ルーティングを直接開いても 404 にならない。
// /worlds をブックマークから開く、リロードするといった操作が普通に起きる。
func TestSPAFallback(t *testing.T) {
	t.Parallel()

	h := spaHandler(assets())

	for _, path := range []string{"/", "/worlds", "/backups", "/deeply/nested/path"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := get(t, h, path)

			if rec.Code != http.StatusOK {
				t.Errorf("HTTP %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "id=root") {
				t.Errorf("index.html が返っていない: %q", rec.Body.String())
			}
		})
	}
}

// 実在するアセットはそのまま返す。
func TestServesRealAssets(t *testing.T) {
	t.Parallel()

	rec := get(t, spaHandler(assets()), "/assets/index-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d", rec.Code)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Errorf("内容が %q", rec.Body.String())
	}
}

// index.html は毎回取り直す。古いものが残ると、新しいアセットを
// 指せずに白い画面になる。
func TestIndexIsNotCached(t *testing.T) {
	t.Parallel()

	h := spaHandler(assets())

	for _, path := range []string{"/", "/worlds"} {
		if got := get(t, h, path).Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s の Cache-Control が %q。no-store のはず", path, got)
		}
	}
}

// ハッシュ付きのアセットは長期キャッシュしてよい。
// Vite が内容のハッシュを名前に埋めるので、中身が変われば名前も変わる。
func TestHashedAssetsAreCached(t *testing.T) {
	t.Parallel()

	got := get(t, spaHandler(assets()), "/assets/index-abc123.js").Header().Get("Cache-Control")
	if !strings.Contains(got, "immutable") || !strings.Contains(got, "max-age=31536000") {
		t.Errorf("Cache-Control が %q", got)
	}
}

// フロントエンドが未ビルドなら、白い画面ではなく理由を返す。
func TestMissingIndexReturnsError(t *testing.T) {
	t.Parallel()

	rec := get(t, spaHandler(fstest.MapFS{}), "/worlds")

	if rec.Code != http.StatusNotFound {
		t.Errorf("HTTP %d。404 のはず", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ビルド") {
		t.Errorf("理由が伝わらない: %q", rec.Body.String())
	}
}

// トークンを localStorage に持つ設計のため、XSS の影響を抑える。
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	h := securityHeaders(spaHandler(assets()))
	header := get(t, h, "/").Header()

	csp := header.Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP に %q が無い: %q", want, csp)
		}
	}

	for key, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
	} {
		if got := header.Get(key); got != want {
			t.Errorf("%s が %q。%q のはず", key, got, want)
		}
	}
}

// 外部への通信を許すと、トークンを盗まれたときに送信先を作られる。
func TestCSPDisallowsExternalSources(t *testing.T) {
	t.Parallel()

	csp := get(t, securityHeaders(spaHandler(assets())), "/").
		Header().Get("Content-Security-Policy")

	if strings.Contains(csp, "*") {
		t.Errorf("CSP にワイルドカードがある: %q", csp)
	}
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP が unsafe-eval を許している: %q", csp)
	}
}
