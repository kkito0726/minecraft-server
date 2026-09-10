package compose

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
)

// psEntry は docker compose ps --format json の 1 件。
//
// 出力の形は Compose のバージョンで変わるため、必要なフィールドだけを拾い、
// 未知のフィールドは無視する。単一オブジェクト・行区切り・配列のいずれでも
// 読めるようにしてある。
type psEntry struct {
	Name      string `json:"Name"`
	State     string `json:"State"`
	Health    string `json:"Health"`
	CreatedAt string `json:"CreatedAt"`
	Image     string `json:"Image"`
}

// Status はコンテナの現在の状態を返す。
//
// 出力を解釈できなくてもエラーにしない。状態表示は画面の入口であり、
// ここで失敗すると管理コンソール全体が使えなくなる。
// 解釈できない場合は「コンテナなし」として扱う。
func (r *Runner) Status(ctx context.Context) (server.ContainerStatus, error) {
	args := append(r.baseArgs(), "ps", "--all", "--format", "json")

	out, err := r.runCapture(ctx, args...)
	if err != nil {
		return server.ContainerStatus{}, err
	}

	entry, found := findService(out, r.cfg.Service, r.cfg.ProjectName)
	if !found {
		return server.ContainerStatus{State: server.ContainerMissing}, nil
	}

	return server.ContainerStatus{
		State:     parseState(entry.State),
		Healthy:   entry.Health == "healthy",
		StartedAt: parseCreatedAt(entry.CreatedAt),
		Image:     entry.Image,
	}, nil
}

// findService は出力から対象サービスの行を探す。
func findService(out, service, projectName string) (psEntry, bool) {
	entries := decodeEntries(out)

	// container_name は compose.yaml で固定されているが、サービス名や
	// プロジェクト名から組み立てた名前になることもある。どれでも拾う。
	candidates := []string{service, projectName, projectName + "-" + service}

	for _, e := range entries {
		for _, c := range candidates {
			if e.Name == c || strings.Contains(e.Name, c) {
				return e, true
			}
		}
	}
	// サービスが 1 つしかない構成なので、名前が一致しなくても
	// 1 件だけならそれを対象とみなす。
	if len(entries) == 1 {
		return entries[0], true
	}
	return psEntry{}, false
}

// decodeEntries は単一オブジェクト・行区切り JSON・配列のいずれも受理する。
func decodeEntries(out string) []psEntry {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}

	// 配列形式
	if strings.HasPrefix(trimmed, "[") {
		var entries []psEntry
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil
		}
		return entries
	}

	// 単一オブジェクトまたは行区切り JSON
	var entries []psEntry
	dec := json.NewDecoder(strings.NewReader(trimmed))
	for {
		var e psEntry
		if err := dec.Decode(&e); err != nil {
			break
		}
		entries = append(entries, e)
	}
	return entries
}

func parseState(s string) server.ContainerState {
	switch strings.ToLower(s) {
	case "running":
		return server.ContainerRunning
	case "exited", "dead", "created":
		return server.ContainerExited
	case "restarting":
		return server.ContainerRestarting
	default:
		// paused など想定外の状態は、操作してよいか判断できないので
		// 「コンテナなし」に倒す。誤って RCON を叩くより安全。
		return server.ContainerMissing
	}
}

// createdAtLayouts は Compose が出す日時の書式。バージョンで揺れる。
var createdAtLayouts = []string{
	"2006-01-02 15:04:05 -0700 MST",
	time.RFC3339,
	time.RFC3339Nano,
}

func parseCreatedAt(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range createdAtLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
