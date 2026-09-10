package world_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// 復元も削除も、元のディレクトリを消さずに退避する。
// 名前の組み立てと解釈が一致していないと、退避したものを一覧に出せなくなる。
func TestQuarantineNameRoundTrip(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 9, 14, 30, 0, 0, time.Local)

	tests := []struct {
		name string
		kind world.QuarantineKind
		want string
	}{
		{"復元による退避", world.QuarantineFromRestore, "world.broken-20260909-143000"},
		{"削除による退避", world.QuarantineFromDelete, "world.deleted-20260909-143000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dirName := world.NewQuarantineName(mustName(t, "world"), tt.kind, at)
			if dirName != tt.want {
				t.Fatalf("組み立てた名前が %q。%q のはず", dirName, tt.want)
			}

			q, err := world.ParseQuarantine(dirName, 1024)
			if err != nil {
				t.Fatalf("解釈に失敗: %v", err)
			}
			if q.OriginalName().String() != "world" {
				t.Errorf("OriginalName() が %q", q.OriginalName())
			}
			if q.Kind() != tt.kind {
				t.Errorf("Kind() が %v", q.Kind())
			}
			if !q.QuarantinedAt().Equal(at) {
				t.Errorf("QuarantinedAt() が %v。%v のはず", q.QuarantinedAt(), at)
			}
			if q.SizeBytes() != 1024 {
				t.Errorf("SizeBytes() が %d", q.SizeBytes())
			}
			if q.DirName() != dirName {
				t.Errorf("DirName() が %q", q.DirName())
			}
			if !q.IsValid() {
				t.Error("IsValid() が偽")
			}
		})
	}
}

func TestParseQuarantineRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"目印が無い", "world"},
		{"通常のワールド名", "creative"},
		{"日時が不正", "world.broken-notatimestamp"},
		{"日時が欠けている", "world.broken-"},
		{"元の名前が不正", "../etc.broken-20260909-143000"},
		{"空", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := world.ParseQuarantine(tt.input, 0); !errors.Is(err, world.ErrInvalidQuarantine) {
				t.Errorf("ErrInvalidQuarantine を期待したが %v", err)
			}
		})
	}
}

// ゼロ値の Quarantine は無効。取り違えて空のパスを組み立てないため。
func TestZeroQuarantineIsInvalid(t *testing.T) {
	t.Parallel()

	var q world.Quarantine
	if q.IsValid() {
		t.Error("ゼロ値が有効になっている")
	}
}

func TestQuarantineKindString(t *testing.T) {
	t.Parallel()

	if got := world.QuarantineFromRestore.String(); got != "復元" {
		t.Errorf("String() が %q", got)
	}
	if got := world.QuarantineFromDelete.String(); got != "削除" {
		t.Errorf("String() が %q", got)
	}
}

// 退避ディレクトリの名前は、通常のワールド名としては受理されない。
// 受理すると退避したものを一覧にワールドとして出してしまう。
func TestQuarantineNameIsNotValidWorldName(t *testing.T) {
	t.Parallel()

	dirName := world.NewQuarantineName(mustName(t, "world"), world.QuarantineFromRestore, time.Now())
	if _, err := world.NewName(dirName); err == nil {
		t.Errorf("%q がワールド名として受理された", dirName)
	}
	if !strings.Contains(dirName, ".broken-") {
		t.Errorf("目印が入っていない: %q", dirName)
	}
}
