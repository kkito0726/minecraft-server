// Package http は管理コンソールの HTTP サーバー。
//
// 静的ファイル（埋め込んだフロントエンド）と /rpc の両方を 1 つの
// ポートで提供する。本番に Node のプロセスは不要。
package http

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// rpcPrefix は Connect のエンドポイントの接頭辞。
const rpcPrefix = "/rpc/"

// UploadBackupPath は zip のアップロードを受け取る場所。
//
// Connect の外側にあるのは、ブラウザが平文の HTTP/2 へ昇格せず
// client-streaming の RPC を使えないため。フロントエンドと合わせる
// 必要があるので公開する。
const UploadBackupPath = "/upload/backup"

// タイムアウト。
//
// WriteTimeout を設けないのは、進捗のストリーミングが分単位で続くため。
// 代わりに ReadHeaderTimeout と IdleTimeout で緩やかに守る。
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

// Config は Server の設定。
type Config struct {
	// Addr は待ち受けアドレス。
	//
	// 既定は 0.0.0.0。Tailscale 経由で別端末のブラウザから開くため
	// （compose.yaml が 25565 を 0.0.0.0 にバインドしているのと同じ理由）。
	// 到達制御は Tailscale、認可は ADMIN_TOKEN が担う。
	Addr string
	// Assets は配信する静的ファイル。
	Assets fs.FS
	// RPC は Connect のハンドラ群。パスは rpcPrefix 配下に置かれる。
	RPC map[string]http.Handler
	// Routes は Connect を通らない通常の HTTP ハンドラ。
	//
	// **認証は呼び出し側で包んでから渡すこと。** ここで自動的には
	// 付けない。付けたつもりで付いていない状態は静かに起きるので、
	// 配線の場所を 1 つに寄せてある（cmd/mcadmind/build.go）。
	Routes map[string]http.Handler
	// Logger は記録先。
	Logger *slog.Logger
}

// Server は管理コンソールの HTTP サーバー。
type Server struct {
	cfg    Config
	logger *slog.Logger
	http   *http.Server
}

// New は Server を作る。
func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		return nil, errors.New("待ち受けアドレスが指定されていません")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	for path, handler := range cfg.RPC {
		mux.Handle(rpcPrefix+strings.TrimPrefix(path, "/"), http.StripPrefix("/rpc", handler))
	}
	// 静的ファイル以外にも同じ防御ヘッダを付ける。JSON を返すので
	// とくに nosniff が要る。
	for path, handler := range cfg.Routes {
		mux.Handle(path, securityHeaders(handler))
	}
	if cfg.Assets != nil {
		mux.Handle("/", securityHeaders(spaHandler(cfg.Assets)))
	}

	// Connect は gRPC 互換のために HTTP/2 を使う。TLS 無しで HTTP/2 を
	// 通すため、平文 HTTP/2（h2c）を有効にする。到達制御は Tailscale が
	// 担うので、この経路自体に TLS は要求しない。
	//
	// golang.org/x/net/http2/h2c は非推奨になったため、標準ライブラリの
	// Protocols で指定する。HTTP/1.1 も残すのは、静的ファイルの配信と
	// ブラウザからの通常のリクエストのため。
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	return &Server{cfg: cfg, logger: logger, http: srv}, nil
}

// ListenAndServe はサーバーを起動する。ctx のキャンセルで穏やかに停止する。
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("管理コンソールを起動しました", "addr", s.cfg.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.shutdown()
	}
}

func (s *Server) shutdown() error {
	s.logger.Info("管理コンソールを停止しています")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("停止できませんでした: %w", err)
	}
	return nil
}
