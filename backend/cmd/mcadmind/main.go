// mcadmind は Minecraft サーバー管理コンソールのバックエンド。
//
// Raspberry Pi 5 上に systemd で常駐し、docker compose と RCON を経由して
// サーバーを操作する。フロントエンドのビルド成果物も自身が配信するため、
// 本番環境に Node のプロセスは不要。
//
// 設定はリポジトリ直下の .env から読む。systemd の EnvironmentFile= は使わない。
// systemd の .env 解釈はシェルのクォート規則と完全には一致せず、
// MC_MOTD="..." のような行や行内の # で挙動が食い違うため
// （どのみち書き込みのために自分で読む必要がある）。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// version はリリースビルド時に -ldflags で埋め込む。
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcadmind:", err)
		os.Exit(1)
	}
}

// run は main から副作用を切り離してテストできるようにしたもの。
func run(args []string, stdout io.Writer) error {
	opts, err := parseFlags(args, stdout)
	if err != nil {
		return err
	}
	if opts.done {
		return nil
	}

	logger := newLogger(opts.logLevel)

	// シグナルで終わる context を先に作る。操作の寿命をこれに紐づけるため、
	// 組み立てより前に用意する必要がある。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := build(ctx, opts, logger)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// errNotConfigured は設定の不足を表す。
var errNotConfigured = errors.New("設定が不足しています")
