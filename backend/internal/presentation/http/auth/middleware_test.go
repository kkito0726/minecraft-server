package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/presentation/http/auth"
)

func middleware(t *testing.T) http.Handler {
	t.Helper()

	interceptor, err := auth.NewInterceptor(token)
	if err != nil {
		t.Fatal(err)
	}
	return interceptor.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

/*
Connect の外側に置くエンドポイントも同じトークンで守る。

ここを通し忘れると、トークン無しで叩ける口がひとつ増えたことに
誰も気づかない。守りが静かに消えるので、試験で固定する。
*/
func TestMiddlewareRejectsMissingToken(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	middleware(t).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("トークン無しで通った: %d", rec.Code)
	}
}

func TestMiddlewareRejectsWrongToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", nil)
	req.Header.Set("Authorization", "Bearer "+token+"x")

	rec := httptest.NewRecorder()
	middleware(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("誤ったトークンで通った: %d", rec.Code)
	}
}

// Bearer 以外の形式も通さない。
func TestMiddlewareRejectsOtherSchemes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{token, "Basic " + token, "bearer " + token} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", nil)
		req.Header.Set("Authorization", value)

		rec := httptest.NewRecorder()
		middleware(t).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%q で通った: %d", value, rec.Code)
		}
	}
}

func TestMiddlewareAcceptsValidToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	middleware(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("正しいトークンで弾かれた: %d", rec.Code)
	}
}
