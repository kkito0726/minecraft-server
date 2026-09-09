package dotenv_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/config/dotenv"
)

// loadFixture は実際の .env.example から作ったフィクスチャを読む。
func loadFixture(t *testing.T) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", "env_sample"))
	if err != nil {
		t.Fatalf("フィクスチャを読めない: %v", err)
	}
	return b
}

func render(t *testing.T, f *dotenv.File) string {
	t.Helper()

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo に失敗: %v", err)
	}
	return buf.String()
}

// diffLines は行単位の差分を返す。順序も含めて比較するため、
// 行が入れ替わっただけでも検出できる。
func diffLines(before, after string) (added, removed []string) {
	b := strings.Split(strings.TrimRight(before, "\n"), "\n")
	a := strings.Split(strings.TrimRight(after, "\n"), "\n")

	countOf := func(lines []string) map[string]int {
		m := make(map[string]int, len(lines))
		for _, l := range lines {
			m[l]++
		}
		return m
	}
	bc, ac := countOf(b), countOf(a)

	// 多重集合の差分。同じ内容の行（空行など）が複数あっても、
	// 増減した分だけを数える。
	remaining := make(map[string]int, len(ac))
	for l, n := range ac {
		if extra := n - bc[l]; extra > 0 {
			remaining[l] = extra
		}
	}
	for _, l := range a {
		if remaining[l] > 0 {
			added = append(added, l)
			remaining[l]--
		}
	}

	remaining = make(map[string]int, len(bc))
	for l, n := range bc {
		if extra := n - ac[l]; extra > 0 {
			remaining[l] = extra
		}
	}
	for _, l := range b {
		if remaining[l] > 0 {
			removed = append(removed, l)
			remaining[l]--
		}
	}
	return added, removed
}

// sourceAndEcho は実際のシェルで source して値を読む。
// 「source できる」ことは仕様であり、自前のパーサでの検証では代用できない。
func sourceAndEcho(t *testing.T, path, key string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	script := "set -a; . " + path + "; set +a; printf '%s' \"$" + key + "\""
	out, err := exec.CommandContext(ctx, "/bin/sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("source に失敗した（生成した .env が壊れている）: %v", err)
	}
	return string(out)
}
