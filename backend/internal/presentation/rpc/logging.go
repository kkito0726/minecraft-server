package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
)

// LoggingInterceptor は想定外のエラーを記録に残す。
//
// toConnectError は内部の詳細（ファイルパスなど）を画面に出さないよう
// メッセージを伏せる。伏せるだけで記録もしないと、「サーバーのログを
// 確認してください」と言いながらログには何も無い状態になり、原因に
// 辿り着く手段がどこにも無くなる。伏せる側と残す側は対で要る。
//
// auth.Interceptor と同じく WrapUnary と WrapStreamingHandler の
// **両方**を実装する。単項だけだと、進捗を配信する WatchOperation の
// 異常だけ記録が残らない。コンパイルは通るので気づけない。
type LoggingInterceptor struct {
	logger *slog.Logger
}

// NewLoggingInterceptor は記録用インターセプタを作る。
func NewLoggingInterceptor(logger *slog.Logger) *LoggingInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoggingInterceptor{logger: logger}
}

// WrapUnary は単項 RPC のエラーを記録する。
func (i *LoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		res, err := next(ctx, req)
		i.record(ctx, req.Spec().Procedure, err)
		return res, err
	}
}

// WrapStreamingHandler はストリーム RPC のエラーを記録する。
func (i *LoggingInterceptor) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, conn)
		i.record(ctx, conn.Spec().Procedure, err)
		return err
	}
}

// WrapStreamingClient はクライアント側。サーバーでは使わないが、
// connect.Interceptor を満たすために必要。
func (i *LoggingInterceptor) WrapStreamingClient(
	next connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return next
}

// record はエラーの重さに応じて記録する。
//
// 利用者が対処できるエラー（実行中、名前の打ち間違い、承諾漏れ）まで
// ERROR で残すと、本当の異常がその中に埋もれる。想定外のものだけを
// ERROR にし、それ以外は追跡用に DEBUG で残す。
func (i *LoggingInterceptor) record(ctx context.Context, procedure string, err error) {
	if err == nil {
		return
	}

	var internal *internalError
	if errors.As(err, &internal) {
		i.logger.ErrorContext(ctx, "想定外のエラーが発生しました",
			"procedure", procedure, "error", internal.Unwrap())
		return
	}

	i.logger.DebugContext(ctx, "エラーを返しました",
		"procedure", procedure, "code", connect.CodeOf(err), "error", err)
}
