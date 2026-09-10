package backup_test

import (
	"slices"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// バックアップ対象は網羅列挙である。ここに無いものは入れない（REQ-004）。
func TestExtraPaths(t *testing.T) {
	t.Parallel()

	got := backup.ExtraPaths()
	want := []string{"plugins", "config", "bukkit.yml", "spigot.yml"}

	if !slices.Equal(got, want) {
		t.Errorf("ExtraPaths() = %v。%v のはず", got, want)
	}
}

// server.properties は rcon.password と management-server-secret を平文で持つ。
// アーカイブに入れると、バックアップを渡した相手にサーバーの操作権を渡すことになる。
func TestExtraPathsExcludesSecrets(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"server.properties",
		"ops.json", "whitelist.json", "banned-players.json", "banned-ips.json",
		"libraries", "versions", "cache", "logs", ".env",
	}

	for _, name := range backup.ExtraPaths() {
		if slices.Contains(forbidden, name) {
			t.Errorf("バックアップ対象に %q が含まれている", name)
		}
	}
}

// 呼び出し側が書き換えても次の呼び出しに影響しない。
func TestExtraPathsIsNotShared(t *testing.T) {
	t.Parallel()

	first := backup.ExtraPaths()
	first[0] = "書き換え"

	if backup.ExtraPaths()[0] != "plugins" {
		t.Error("ExtraPaths() が内部の配列を共有している")
	}
}

// 除外するファイル名。session.lock は起動時に作り直されるので持ち込まない。
func TestExcludedNames(t *testing.T) {
	t.Parallel()

	if !slices.Contains(backup.ExcludedNames(), "session.lock") {
		t.Errorf("ExcludedNames() = %v。session.lock を含むはず", backup.ExcludedNames())
	}
}
