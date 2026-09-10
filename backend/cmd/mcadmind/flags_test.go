package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlags(t *testing.T) {
	t.Parallel()

	t.Run("-version は表示して正常終了する", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		opts, err := parseFlags([]string{"-version"}, &out)
		if err != nil {
			t.Fatalf("エラーが出ないはずが %v", err)
		}
		if !opts.done {
			t.Error("done が真になるはず")
		}
		if !strings.Contains(out.String(), "mcadmind") {
			t.Errorf("バージョンを出力していない: %q", out.String())
		}
	})

	t.Run("project-dir は絶対パスに解決される", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		opts, err := parseFlags([]string{"-project-dir", "."}, &out)
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(opts.projectDir) {
			t.Errorf("絶対パスになっていない: %q", opts.projectDir)
		}
	})

	t.Run("未知のフラグは拒否される", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		if _, err := parseFlags([]string{"-nonexistent"}, &out); err == nil {
			t.Error("エラーになるはず")
		}
	})
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	valid := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"INFO":  slog.LevelInfo,
	}
	for in, want := range valid {
		got, err := parseLogLevel(in)
		if err != nil {
			t.Errorf("%q が拒否された: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q → %v。%v のはず", in, got, want)
		}
	}

	if _, err := parseLogLevel("verbose"); err == nil {
		t.Error("不正な値は拒否されるはず")
	}
}

// systemd 配下では作業ディレクトリがリポジトリと一致しない。
// 検証が壊れていると docker compose が分かりにくい形で失敗する。
func TestValidateProjectDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   []string
		wantErr string
	}{
		{name: "両方そろっている", files: []string{"compose.yaml", ".env"}},
		{name: "compose.yaml がない", files: []string{".env"}, wantErr: "compose.yaml"},
		{name: ".env がない", files: []string{"compose.yaml"}, wantErr: ".env"},
		{name: "どちらもない", wantErr: "compose.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			err := validateProjectDir(dir)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("エラーが出ないはずが %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("エラーに %q を期待したが %v", tt.wantErr, err)
			}
		})
	}
}

// project-dir が不正なら起動しない。
func TestRunRejectsInvalidProjectDir(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := run([]string{"-project-dir", t.TempDir()}, &out)
	if err == nil || !strings.Contains(err.Error(), "compose.yaml") {
		t.Fatalf("compose.yaml のエラーを期待したが %v", err)
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run([]string{"-version"}, &out); err != nil {
		t.Fatalf("エラーが出ないはずが %v", err)
	}
	if !strings.Contains(out.String(), "mcadmind") {
		t.Errorf("出力が %q", out.String())
	}
}

func TestRunRejectsInvalidLogLevel(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run([]string{"-log-level", "verbose"}, &out); err == nil {
		t.Error("エラーになるはず")
	}
}
