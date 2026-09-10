package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/http/auth"
)

const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// stubService は認証だけを試すための最小の実装。
type stubService struct {
	mcadminv1connect.UnimplementedOperationServiceHandler
	unaryCalled  bool
	streamCalled bool
}

func (s *stubService) GetActiveOperation(
	context.Context,
	*connect.Request[mcadminv1.GetActiveOperationRequest],
) (*connect.Response[mcadminv1.GetActiveOperationResponse], error) {
	s.unaryCalled = true
	return connect.NewResponse(&mcadminv1.GetActiveOperationResponse{Present: false}), nil
}

func (s *stubService) WatchOperation(
	_ context.Context,
	_ *connect.Request[mcadminv1.WatchOperationRequest],
	stream *connect.ServerStream[mcadminv1.WatchOperationResponse],
) error {
	s.streamCalled = true
	return stream.Send(&mcadminv1.WatchOperationResponse{Seq: 1})
}

func newServer(t *testing.T, svc *stubService) (*httptest.Server, mcadminv1connect.OperationServiceClient) {
	t.Helper()

	interceptor, err := auth.NewInterceptor(token)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	path, handler := mcadminv1connect.NewOperationServiceHandler(
		svc, connect.WithInterceptors(interceptor))
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := mcadminv1connect.NewOperationServiceClient(srv.Client(), srv.URL)
	return srv, client
}

// clientAuth はヘッダを付けるクライアント側インターセプタ。
//
// 単項用（connect.UnaryInterceptorFunc）だけではストリーム RPC に
// ヘッダが付かない。サーバー側の落とし穴とちょうど対になっている。
type clientAuth struct{ value string }

func (c clientAuth) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if c.value != "" {
			req.Header().Set("Authorization", c.value)
		}
		return next(ctx, req)
	}
}

func (c clientAuth) WrapStreamingClient(
	next connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if c.value != "" {
			conn.RequestHeader().Set("Authorization", c.value)
		}
		return conn
	}
}

func (c clientAuth) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return next
}

func withToken(t *testing.T, value string) connect.ClientOption {
	t.Helper()

	return connect.WithInterceptors(clientAuth{value: value})
}

func TestUnaryAcceptsValidToken(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	srv, _ := newServer(t, svc)

	client := mcadminv1connect.NewOperationServiceClient(
		srv.Client(), srv.URL, withToken(t, "Bearer "+token))

	if _, err := client.GetActiveOperation(
		context.Background(),
		connect.NewRequest(&mcadminv1.GetActiveOperationRequest{}),
	); err != nil {
		t.Fatalf("通るはずが %v", err)
	}
	if !svc.unaryCalled {
		t.Error("ハンドラが呼ばれていない")
	}
}

func TestUnaryRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
	}{
		{"ヘッダなし", ""},
		{"空のヘッダ", " "},
		{"Bearer なしの生の値", token},
		{"誤ったトークン", "Bearer " + strings.Repeat("f", len(token))},
		{"短いトークン", "Bearer short"},
		{"接頭辞が違う", "Basic " + token},
		{"トークンが空", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := &stubService{}
			srv, _ := newServer(t, svc)
			client := mcadminv1connect.NewOperationServiceClient(
				srv.Client(), srv.URL, withToken(t, tt.header))

			_, err := client.GetActiveOperation(
				context.Background(),
				connect.NewRequest(&mcadminv1.GetActiveOperationRequest{}))
			if err == nil {
				t.Fatal("拒否されるはず")
			}
			if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
				t.Errorf("コードが %v。Unauthenticated のはず", got)
			}
			if svc.unaryCalled {
				t.Error("拒否したのにハンドラが呼ばれている")
			}
		})
	}
}

// Connect の単項用インターセプタだけを実装すると、ストリーム RPC が
// 認証を経由せず素通りする。実装上の落とし穴なので回帰テストで固定する。
func TestStreamingIsAuthenticated(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	srv, client := newServer(t, svc)
	_ = srv

	stream, err := client.WatchOperation(
		context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: "x"}))
	if err != nil {
		// 接続時点で弾かれる場合もある
		if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
			t.Fatalf("コードが %v", got)
		}
		if svc.streamCalled {
			t.Error("拒否したのにハンドラが呼ばれている")
		}
		return
	}

	// 受信時に弾かれる場合
	for stream.Receive() {
	}
	streamErr := stream.Err()
	if streamErr == nil {
		t.Fatal("ストリームが認証を素通りしている")
	}
	if got := connect.CodeOf(streamErr); got != connect.CodeUnauthenticated {
		t.Errorf("コードが %v。Unauthenticated のはず", got)
	}
	if svc.streamCalled {
		t.Error("拒否したのにハンドラが呼ばれている")
	}
}

func TestStreamingAcceptsValidToken(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	srv, _ := newServer(t, svc)

	client := mcadminv1connect.NewOperationServiceClient(
		srv.Client(), srv.URL, withToken(t, "Bearer "+token))

	stream, err := client.WatchOperation(
		context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: "x"}))
	if err != nil {
		t.Fatalf("通るはずが %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("ストリームが失敗した: %v", err)
	}
	if !svc.streamCalled {
		t.Error("ハンドラが呼ばれていない")
	}
}

// 短いトークンは起動時に弾く。運用で「とりあえず短い値」を
// 設定してしまう事故を防ぐ。
func TestNewInterceptorRejectsShortToken(t *testing.T) {
	t.Parallel()

	for _, tok := range []string{"", "short", strings.Repeat("a", 31)} {
		_, err := auth.NewInterceptor(tok)
		if err == nil {
			t.Errorf("%d 文字は拒否されるはず", len(tok))
			continue
		}
		// エラーメッセージに生成方法が出ること。自己解決できるように。
		if !strings.Contains(err.Error(), "openssl rand -hex 32") {
			t.Errorf("エラーに生成コマンドが含まれていない: %v", err)
		}
		if !errors.Is(err, auth.ErrWeakToken) {
			t.Errorf("ErrWeakToken を期待したが %v", err)
		}
	}

	if _, err := auth.NewInterceptor(strings.Repeat("a", 32)); err != nil {
		t.Errorf("32 文字は受理されるはずが %v", err)
	}
}
