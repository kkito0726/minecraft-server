// Package auth は管理コンソールの認証を行う。
//
// Tailscale による到達制御の上に、共有トークンによる認可を重ねる二段構え。
// docs/raspberry-pi.md が「tailnet に招いた全員が 25565 に到達できる」ため
// MC_WHITELIST を別レイヤーの制御として設けているのと同じ考え方で、
// 管理画面はワールドの削除まで可能なぶん、より強い制御が要る。
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// ErrWeakToken はトークンが短すぎることを表す。
var ErrWeakToken = errors.New("ADMIN_TOKEN が短すぎます")

// minTokenLength はトークンの最小長。
// openssl rand -hex 32 が 64 文字を出すので、その半分を下限にする。
const minTokenLength = 32

// bearerPrefix は Authorization ヘッダの接頭辞。
const bearerPrefix = "Bearer "

// Interceptor は Connect の認証インターセプタ。
//
// WrapUnary と WrapStreamingHandler の**両方**を実装する。
// connect.UnaryInterceptorFunc だけを使うと、単項 RPC しか認証されず、
// 進捗を配信する WatchOperation が素通りする。コンパイルは通るため
// 気づきにくい。
type Interceptor struct {
	token []byte
}

// NewInterceptor は認証インターセプタを作る。
//
// トークンが短ければ起動時にエラーにする。運用で「とりあえず短い値」を
// 設定してしまう事故を防ぐため、エラーには生成コマンドを含める。
func NewInterceptor(token string) (*Interceptor, error) {
	if len(token) < minTokenLength {
		return nil, fmt.Errorf(
			"%w（%d 文字）。%d 文字以上にしてください。生成: openssl rand -hex 32",
			ErrWeakToken, len(token), minTokenLength)
	}
	return &Interceptor{token: []byte(token)}, nil
}

// WrapUnary は単項 RPC を認証する。
func (i *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.check(req.Header().Get("Authorization")); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingHandler はストリーム RPC を認証する。
//
// これが無いと WatchOperation が認証を経由しない。
func (i *Interceptor) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.check(conn.RequestHeader().Get("Authorization")); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// WrapStreamingClient はクライアント側。サーバーでは使わないが、
// connect.Interceptor を満たすために必要。
func (i *Interceptor) WrapStreamingClient(
	next connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return next
}

// check は Authorization ヘッダを検証する。
//
// 比較は定数時間で行う。長さの違いで早期に返すと、
// 応答時間からトークンの長さが推測できる。
func (i *Interceptor) check(header string) error {
	value, ok := strings.CutPrefix(header, bearerPrefix)
	if !ok || value == "" {
		return connect.NewError(connect.CodeUnauthenticated,
			errors.New("認証が必要です"))
	}
	if subtle.ConstantTimeCompare([]byte(value), i.token) != 1 {
		return connect.NewError(connect.CodeUnauthenticated,
			errors.New("トークンが正しくありません"))
	}
	return nil
}

var _ connect.Interceptor = (*Interceptor)(nil)
