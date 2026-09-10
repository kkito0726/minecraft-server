package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
)

type options struct {
	projectDir string
	logLevel   slog.Level
	// done は -version のように「表示して正常終了する」場合に真になる。
	done bool
}

func parseFlags(args []string, stdout io.Writer) (options, error) {
	fs := flag.NewFlagSet("mcadmind", flag.ContinueOnError)
	fs.SetOutput(stdout)

	projectDir := fs.String("project-dir", ".", "compose.yaml と .env があるディレクトリ")
	logLevel := fs.String("log-level", "info", "ログの詳細度 (debug / info / warn / error)")
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

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		return options{}, err
	}
	return options{projectDir: abs, logLevel: level}, nil
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log-level が不正です: %q (debug / info / warn / error)", s)
	}
}
