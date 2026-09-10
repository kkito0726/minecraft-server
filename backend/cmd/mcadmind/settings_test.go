package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/config/dotenv"
)

// writeEnv は .env を用意する。
func writeEnv(t *testing.T, lines string) (string, *dotenv.Adapter) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, dotenv.NewAdapter(path)
}

// loadFrom は一時的な .env から設定を読む。
func loadFrom(t *testing.T, lines string) (settings, string, error) {
	t.Helper()

	dir, config := writeEnv(t, lines)
	s, err := loadSettings(context.Background(), dir, config)
	return s, dir, err
}

// ADMIN_TOKEN が無ければ起動を止め、生成方法を案内する。
//
// build() を経由せず直接呼ぶのは、フロントエンドがビルド済みかどうかで
// 到達するかが変わり、カバレッジが環境に依存してしまうため。
func TestLoadSettingsRequiresToken(t *testing.T) {
	t.Parallel()

	for _, env := range []string{
		"MC_VERSION=26.2\n",
		"MC_VERSION=26.2\nADMIN_TOKEN=\n",
	} {
		_, _, err := loadFrom(t, env)
		if err == nil {
			t.Fatal("エラーになるはず")
		}
		if !strings.Contains(err.Error(), "ADMIN_TOKEN") {
			t.Errorf("エラーに ADMIN_TOKEN が含まれていない: %v", err)
		}
		if !strings.Contains(err.Error(), "openssl rand -hex 32") {
			t.Errorf("生成方法が案内されていない: %v", err)
		}
	}
}

// 既定値が入る。ADMIN_ADDR は 0.0.0.0 で、Tailscale 経由の別端末から開ける。
func TestLoadSettingsDefaults(t *testing.T) {
	t.Parallel()

	got, _, err := loadFrom(t, "ADMIN_TOKEN="+strings.Repeat("a", 64)+"\n")
	if err != nil {
		t.Fatalf("loadSettings に失敗: %v", err)
	}

	if got.addr != defaultAddr {
		t.Errorf("addr が %q。%q のはず", got.addr, defaultAddr)
	}
	if !strings.HasPrefix(got.addr, "0.0.0.0") {
		t.Errorf("既定が %q。ループバックだと別端末から開けない", got.addr)
	}
	if got.dockerBin == "" {
		t.Error("docker のパスが解決されていない")
	}
	if !filepath.IsAbs(got.dockerBin) {
		t.Errorf("docker のパスが絶対パスでない: %q。systemd 配下では PATH が通らない", got.dockerBin)
	}
}

func TestLoadSettingsOverrides(t *testing.T) {
	t.Parallel()

	env := "ADMIN_TOKEN=" + strings.Repeat("b", 64) + "\n" +
		"ADMIN_ADDR=127.0.0.1:9999\n"

	got, _, err := loadFrom(t, env)
	if err != nil {
		t.Fatal(err)
	}
	if got.addr != "127.0.0.1:9999" {
		t.Errorf("addr が %q", got.addr)
	}
}

// docker が見つからなければ起動時に止める。
// 実行のたびに失敗するより、起動時に分かる方がよい。
func TestLoadSettingsRequiresDocker(t *testing.T) {
	t.Parallel()

	env := "ADMIN_TOKEN=" + strings.Repeat("c", 64) + "\n" +
		"ADMIN_DOCKER_BIN=definitely-not-a-real-binary-name\n"

	_, _, err := loadFrom(t, env)
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("エラーに docker が含まれていない: %v", err)
	}
}

// .env が読めなければエラーにする。
func TestLoadSettingsMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	adapter := dotenv.NewAdapter(filepath.Join(dir, ".env"))
	if _, err := loadSettings(context.Background(), dir, adapter); err == nil {
		t.Error("エラーになるはず")
	}
}

// 公開してよい設定のキーに秘匿情報を含めない。
//
// 許可リスト方式にしているので、新しいキーが増えても既定で秘匿される。
func TestPublicConfigKeysExcludeSecrets(t *testing.T) {
	t.Parallel()

	forbidden := []string{"ADMIN_TOKEN", "RCON_PASSWORD", "MC_SEED"}
	for _, key := range publicConfigKeys {
		for _, bad := range forbidden {
			if key == bad {
				t.Errorf("%s が公開対象に含まれている", key)
			}
		}
	}
	if len(publicConfigKeys) == 0 {
		t.Error("公開対象が空")
	}
}

// インフラの組み立てが通る。
func TestBuildInfra(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := buildInfra(options{projectDir: dir}, settings{
		dockerBin: "/bin/echo",
		backupDir: filepath.Join(dir, "backups"),
	})
	if err != nil {
		t.Fatalf("buildInfra に失敗: %v", err)
	}
	if got.runtime == nil || got.console == nil || got.levels == nil ||
		got.worlds == nil || got.lock == nil || got.backups == nil {
		t.Errorf("組み立てが不完全: %+v", got)
	}
	if got.backups.Directory() != filepath.Join(dir, "backups") {
		t.Errorf("保管先が %q", got.backups.Directory())
	}
}

// 保管先の既定は プロジェクトディレクトリ配下の backups/。
//
// 相対パスをプロセスの作業ディレクトリ基準で解決すると、systemd 配下で
// 思わぬ場所にバックアップが溜まる。基準は必ずプロジェクトディレクトリ。
func TestLoadSettingsResolvesBackupDir(t *testing.T) {
	t.Parallel()

	token := "ADMIN_TOKEN=" + strings.Repeat("d", 64) + "\n"

	got, dir, err := loadFrom(t, token)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "backups"); got.backupDir != want {
		t.Errorf("backupDir が %q。%q のはず", got.backupDir, want)
	}

	got, dir, err = loadFrom(t, token+"ADMIN_BACKUP_DIR=保管\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "保管"); got.backupDir != want {
		t.Errorf("backupDir が %q。%q のはず", got.backupDir, want)
	}

	got, _, err = loadFrom(t, token+"ADMIN_BACKUP_DIR=/mnt/usb/backups\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.backupDir != "/mnt/usb/backups" {
		t.Errorf("絶対パスが %q。そのまま使うはず", got.backupDir)
	}
}
