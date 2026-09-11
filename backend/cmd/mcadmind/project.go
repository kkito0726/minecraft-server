package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

/*
compose のプロジェクト名は、指し示されたディレクトリから決める。

固定の定数にしていると、-project-dir を別のディレクトリへ向けても
**同じプロジェクト**を操作することになる。テスト用の環境に向けたつもりの
mcadmind が本番のコンテナを down させる。

名前を明示すること自体は必要で、省略すると Compose はディレクトリ名から
推測するため、systemd 配下から叩いたときに手動操作とは別のコンテナが
二重に立ち上がる。だから「明示する」と「ディレクトリごとに変える」の
両方を満たす必要がある。

決め方は上から順に:
 1. .env の ADMIN_COMPOSE_PROJECT
 2. <project-dir>/compose.yaml の name:
 3. 既定値

2 を見るのは、その値こそが「このディレクトリのプロジェクト名」だから。
人が 1 を設定し忘れても正しい方へ倒れる。
*/
const defaultComposeProject = "minecraft-server"

func composeProjectFrom(projectDir, configured string) string {
	if configured != "" {
		return configured
	}
	if name := composeNameIn(projectDir); name != "" {
		return name
	}
	return defaultComposeProject
}

// composeNameIn は compose.yaml の最上位の name: を読む。
//
// YAML として解釈しないのは、必要なのがこの 1 行だけで、依存を増やす
// 理由が無いため。字下げのある name: は services の中の別物なので拾わない。
func composeNameIn(projectDir string) string {
	f, err := os.Open(filepath.Join(projectDir, "compose.yaml"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// 字下げされていない行だけが最上位のキー。
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		rest, ok := strings.CutPrefix(line, "name:")
		if !ok {
			continue
		}
		return cleanScalar(rest)
	}
	return ""
}

// cleanScalar は行末のコメントと引用符を落とす。
func cleanScalar(value string) string {
	if i := strings.Index(value, " #"); i >= 0 {
		value = value[:i]
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
