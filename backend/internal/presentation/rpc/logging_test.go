package rpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
)

// failingService は好きなエラーを返すだけのサービス。
//
// インターセプタを通した実物のサーバーで確かめる。connect.AnyRequest は
// 外部から実装できないため、偽物を差し込む方法が無い。
type failingService struct {
	mcadminv1connect.UnimplementedOperationServiceHandler
	err error
}

func (s *failingService) GetOperation(
	context.Context, *connect.Request[mcadminv1.GetOperationRequest],
) (*connect.Response[mcadminv1.GetOperationResponse], error) {
	if s.err != nil {
		return nil, toConnectError(s.err)
	}
	return connect.NewResponse(&mcadminv1.GetOperationResponse{}), nil
}

func (s *failingService) WatchOperation(
	_ context.Context,
	_ *connect.Request[mcadminv1.WatchOperationRequest],
	_ *connect.ServerStream[mcadminv1.WatchOperationResponse],
) error {
	return toConnectError(s.err)
}

// newLoggingServer は記録先つきのサーバーを 1 つ立てる。
func newLoggingServer(t *testing.T, failWith error) (*bytes.Buffer, mcadminv1connect.OperationServiceClient) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mux := http.NewServeMux()
	path, handler := mcadminv1connect.NewOperationServiceHandler(
		&failingService{err: failWith},
		connect.WithInterceptors(NewLoggingInterceptor(logger)))
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &buf, mcadminv1connect.NewOperationServiceClient(srv.Client(), srv.URL)
}

/*
想定外のエラーは画面に詳細を出さない。その代わり必ず記録に残す。

これが無いと「サーバーのログを確認してください」と言いながら
ログには何も無い状態になり、原因に辿り着く手段が消える。
実際、外部の zip を読ませたときにこれで詰まった。
*/
func TestLoggingInterceptorRecordsInternalCause(t *testing.T) {
	t.Parallel()

	cause := errors.New(`"data/" は data/ 配下ではありません`)
	buf, client := newLoggingServer(t, cause)

	_, err := client.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: "op-1"}))
	if err == nil {
		t.Fatal("エラーが返っていない")
	}

	logged := buf.String()
	if !strings.Contains(logged, "data/ 配下ではありません") {
		t.Errorf("原因がログに出ていない: %s", logged)
	}
	if !strings.Contains(logged, "GetOperation") {
		t.Errorf("どの RPC かがログに出ていない: %s", logged)
	}
}

// 記録には残すが、画面には伏せる。両立していることを確かめる。
func TestInternalCauseIsNotSentToClient(t *testing.T) {
	t.Parallel()

	secret := "/home/pi/minecraft-server/data/world/level.dat"
	err := toConnectError(errors.New("読めません: " + secret))

	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Fatalf("コードが internal ではない: %v", got)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("内部の詳細が画面向けのメッセージに漏れている: %s", err.Error())
	}

	var internal *internalError
	if !errors.As(err, &internal) {
		t.Fatal("原因を取り出せない。記録に残せなくなる")
	}
	if !strings.Contains(internal.Unwrap().Error(), secret) {
		t.Errorf("原因が失われている: %v", internal.Unwrap())
	}
}

// 画面に出るメッセージが、通信を経ても伏せられたままであること。
func TestClientNeverSeesInternalDetail(t *testing.T) {
	t.Parallel()

	secret := "/home/pi/secret-path/level.dat"
	_, client := newLoggingServer(t, errors.New("読めません: "+secret))

	_, err := client.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: "op-1"}))
	if err == nil {
		t.Fatal("エラーが返っていない")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("内部の詳細がクライアントまで届いている: %s", err.Error())
	}
}

// 利用者が対処できるエラーまで ERROR で記録すると、本当の異常が埋もれる。
func TestExpectedErrorsAreNotLoggedAsErrors(t *testing.T) {
	t.Parallel()

	buf, client := newLoggingServer(t, operations.ErrBusy)

	if _, err := client.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: "op-1"})); err == nil {
		t.Fatal("エラーが返っていない")
	}
	if strings.Contains(buf.String(), "level=ERROR") {
		t.Errorf("想定内のエラーが ERROR で記録されている: %s", buf.String())
	}
}

func TestSuccessIsNotLogged(t *testing.T) {
	t.Parallel()

	buf, client := newLoggingServer(t, nil)

	if _, err := client.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: "op-1"})); err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if strings.Contains(buf.String(), "level=ERROR") {
		t.Errorf("成功したのに ERROR が記録されている: %s", buf.String())
	}
}

/*
auth.go と同じ罠。WrapUnary だけ実装すると WatchOperation の
エラーが素通りし、進捗の配信で起きた異常だけ記録が残らない。
コンパイルは通るので気づけない。
*/
func TestLoggingInterceptorCoversStreamingHandler(t *testing.T) {
	t.Parallel()

	cause := errors.New("購読の配信に失敗しました")
	buf, client := newLoggingServer(t, cause)

	stream, err := client.WatchOperation(context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: "op-1"}))
	if err != nil {
		t.Fatal(err)
	}
	for stream.Receive() {
		// 受け取るものは無い。エラーになるまで回す。
	}
	if stream.Err() == nil {
		t.Fatal("ストリームがエラーで終わっていない")
	}

	if !strings.Contains(buf.String(), cause.Error()) {
		t.Errorf("ストリームの原因がログに出ていない: %s", buf.String())
	}
}
