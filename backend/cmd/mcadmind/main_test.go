package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// systemd 配下では作業ディレクトリがリポジトリと一致しないため、
// project-dir の検証が壊れていると docker compose が分かりにくい形で失敗する。
func TestValidateProjectDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   []string
		wantErr string
	}{
		{name: "両方そろっている", files: []string{"compose.yaml", ".env"}},
		{name: "compose.yaml がない", files: []string{".env"}, wantErr: "compose.yaml が見つかりません"},
		{name: ".env がない", files: []string{"compose.yaml"}, wantErr: ".env が見つかりません"},
		{name: "どちらもない", files: nil, wantErr: "compose.yaml が見つかりません"},
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
			t.Fatalf("エラーが出ないはずが %v", err)
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

func TestRun(t *testing.T) {
	t.Parallel()

	built := deps{webuiBuilt: func() bool { return true }}
	notBuilt := deps{webuiBuilt: func() bool { return false }}

	// compose.yaml と .env がそろったディレクトリを用意する
	projectDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for _, f := range []string{"compose.yaml", ".env"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	t.Run("-version は何もせず終わる", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		if err := run([]string{"-version"}, &out, built); err != nil {
			t.Fatalf("エラーが出ないはずが %v", err)
		}
	})

	t.Run("引数が不正なら起動しない", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		if err := run([]string{"-nonexistent"}, &out, built); err == nil {
			t.Error("エラーになるはず")
		}
	})

	t.Run("project-dir が不正なら起動しない", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		err := run([]string{"-project-dir", t.TempDir()}, &out, built)
		if err == nil || !strings.Contains(err.Error(), "compose.yaml が見つかりません") {
			t.Fatalf("compose.yaml のエラーを期待したが %v", err)
		}
	})

	// フロントエンドを埋め込み忘れたまま起動すると白い画面になり原因が分かりにくい。
	// 起動時に検出してビルド手順を案内する。
	t.Run("フロントエンドが未ビルドなら案内して終わる", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		err := run([]string{"-project-dir", projectDir(t)}, &out, notBuilt)
		if err == nil || !strings.Contains(err.Error(), "make build-front") {
			t.Fatalf("ビルド手順の案内を期待したが %v", err)
		}
	})

	t.Run("そろっていればフェーズ 9 の未実装まで到達する", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		err := run([]string{"-project-dir", projectDir(t)}, &out, built)
		if !errors.Is(err, errNotImplemented) {
			t.Fatalf("errNotImplemented を期待したが %v", err)
		}
	})
}
