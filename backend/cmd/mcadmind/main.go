// mcadmind は Minecraft サーバー管理コンソールのバックエンド。
//
// Raspberry Pi 5 上に systemd で常駐し、docker compose と RCON を経由して
// サーバーを操作する。フロントエンドのビルド成果物も自身が配信するため、
// 本番環境に Node のプロセスは不要。
//
// 設定はリポジトリ直下の .env から読む。systemd の EnvironmentFile= は使わない。
// systemd の .env 解釈はシェルのクォート規則と一致せず、MC_MOTD="..." のような行や
// 行内の # で挙動が食い違うため（どのみち書き込みのために自分で読む必要がある）。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kkito0726/minecraft-server/backend/internal/webui"
)

// version はリリースビルド時に -ldflags で埋め込む。
var version = "dev"

// errNotImplemented はフェーズ 9 で HTTP サーバーの起動に置き換わる。
var errNotImplemented = errors.New("未実装です。現在はプロジェクトの雛形のみです")

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcadmind:", err)
		os.Exit(1)
	}
}

// run は main から副作用を切り離してテストできるようにしたもの。
// 引数と出力先を受け取り、プロセスの終了は呼び出し側に任せる。
func run(args []string, stdout io.Writer) error {
	opts, err := parseFlags(args, stdout)
	if err != nil {
		return err
	}
	if opts.done {
		return nil
	}

	if err := validateProjectDir(opts.projectDir); err != nil {
		return err
	}

	// フロントエンドが埋め込まれているかを先に確かめる。ビルドを忘れたまま
	// 起動すると白い画面になり、原因が分かりにくいため。
	if !webui.IsBuilt() {
		return errors.New("フロントエンドがビルドされていません。make build-front を実行してください")
	}

	// TODO(フェーズ 9): 依存の組み立てと HTTP サーバーの起動をここに実装する。
	return fmt.Errorf("%w（project-dir=%s）", errNotImplemented, opts.projectDir)
}

type options struct {
	projectDir string
	// done は -version のように「表示して正常終了する」場合に真になる。
	done bool
}

func parseFlags(args []string, stdout io.Writer) (options, error) {
	fs := flag.NewFlagSet("mcadmind", flag.ContinueOnError)
	fs.SetOutput(stdout)
	projectDir := fs.String("project-dir", ".", "compose.yaml と .env があるディレクトリ")
	showVersion := fs.Bool("version", false, "バージョンを表示して終了する")

	if err := fs.Parse(args); err != nil {
		return options{}, fmt.Errorf("引数を解釈できません: %w", err)
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, "mcadmind", version); err != nil {
			return options{}, fmt.Errorf("バージョンを出力できません: %w", err)
		}
		return options{done: true}, nil
	}

	abs, err := filepath.Abs(*projectDir)
	if err != nil {
		return options{}, fmt.Errorf("project-dir の絶対パスを解決できません: %w", err)
	}
	return options{projectDir: abs}, nil
}

// validateProjectDir は compose.yaml と .env の存在を確認する。
// systemd 配下では作業ディレクトリがリポジトリと一致しないため、
// 起動時に確かめておかないと後続の docker compose が分かりにくい形で失敗する。
func validateProjectDir(dir string) error {
	for _, name := range []string{"compose.yaml", ".env"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("%s が見つかりません (%s): %w", name, dir, err)
		}
	}
	return nil
}
