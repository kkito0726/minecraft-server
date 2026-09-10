package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"
)

func TestNewRequiresAddr(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{}); err == nil {
		t.Error("待ち受けアドレスが無ければエラーになるはず")
	}
}

// RPC のハンドラは /rpc 配下に置かれる。
func TestRPCHandlersAreMounted(t *testing.T) {
	t.Parallel()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// StripPrefix 後のパスがハンドラの想定どおりであること
		if r.URL.Path != "/mcadmin.v1.TestService/Method" {
			t.Errorf("ハンドラに渡ったパスが %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	srv, err := New(Config{
		Addr:   "127.0.0.1:0",
		Assets: fstest.MapFS{"index.html": {Data: []byte("x")}},
		RPC:    map[string]http.Handler{"/mcadmin.v1.TestService/": handler},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/rpc/mcadmin.v1.TestService/Method", nil))

	if !called {
		t.Error("RPC ハンドラが呼ばれていない")
	}
}

// 平文 HTTP/2 を有効にする。Connect は gRPC 互換のために HTTP/2 を使う。
func TestUnencryptedHTTP2IsEnabled(t *testing.T) {
	t.Parallel()

	srv, err := New(Config{Addr: "127.0.0.1:0", Assets: fstest.MapFS{}})
	if err != nil {
		t.Fatal(err)
	}

	if srv.http.Protocols == nil {
		t.Fatal("Protocols が設定されていない")
	}
	if !srv.http.Protocols.UnencryptedHTTP2() {
		t.Error("平文 HTTP/2 が無効。Connect のストリーミングが動かない")
	}
	// 静的ファイルの配信のために HTTP/1.1 も残す
	if !srv.http.Protocols.HTTP1() {
		t.Error("HTTP/1.1 が無効")
	}
}

// ヘッダの読み取りにはタイムアウトを設ける。
// 書き込みには設けない。進捗のストリーミングが分単位で続くため。
func TestTimeouts(t *testing.T) {
	t.Parallel()

	srv, err := New(Config{Addr: "127.0.0.1:0", Assets: fstest.MapFS{}})
	if err != nil {
		t.Fatal(err)
	}

	if srv.http.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout が無い")
	}
	if srv.http.WriteTimeout != 0 {
		t.Error("WriteTimeout が設定されている。長時間のストリーミングが切れる")
	}
}

// ctx のキャンセルで穏やかに停止する。
func TestListenAndServeStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	srv, err := New(Config{
		Addr:   "127.0.0.1:0",
		Assets: fstest.MapFS{"index.html": {Data: []byte("x")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("停止時にエラー: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("停止しない")
	}
}
