package backup_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// ファイル名にバージョンとワールド名を入れる。
//
// docs/backup-restore.md が「ワールドのアップグレードは片道」なので
// バージョンを入れると定めている。ワールド名は複数ワールド対応で追加した。
func TestBuildName(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 10, 14, 30, 0, 0, time.Local)

	tests := []struct {
		name    string
		version string
		level   string
		note    string
		want    string
	}{
		{
			name:    "メモなし",
			version: "26.2", level: "world",
			want: "backup-26.2-world-20260910-143000.zip",
		},
		{
			name:    "メモあり",
			version: "26.2", level: "creative", note: "before upgrade",
			want: "backup-26.2-creative-20260910-143000-before-upgrade.zip",
		},
		{
			name:    "メモの記号は落とす",
			version: "26.2", level: "world", note: "v1.0 / テスト!",
			want: "backup-26.2-world-20260910-143000-v1-0.zip",
		},
		{
			name:    "メモが記号だけなら付けない",
			version: "26.2", level: "world", note: "!!!",
			want: "backup-26.2-world-20260910-143000.zip",
		},
		{
			name:    "バージョンが空でも壊れない",
			version: "", level: "world",
			want: "backup-unknown-world-20260910-143000.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := backup.BuildName(tt.version, tt.level, tt.note, at)
			if got != tt.want {
				t.Errorf("BuildName() = %q。%q のはず", got, tt.want)
			}
			// 組み立てたものは必ず ID として受理される
			if _, err := backup.NewID(got); err != nil {
				t.Errorf("組み立てた名前が ID として不正: %v", err)
			}
		})
	}
}

// メモが長すぎるとファイル名が扱いにくくなる。
func TestBuildNameTruncatesNote(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 10, 14, 30, 0, 0, time.Local)
	got := backup.BuildName("26.2", "world", strings.Repeat("a", 200), at)

	if len(got) > 120 {
		t.Errorf("ファイル名が %d 文字。長すぎる: %q", len(got), got)
	}
	if _, err := backup.NewID(got); err != nil {
		t.Errorf("ID として不正: %v", err)
	}
}

// ファイル名から推測した値は「参考」であって真の値ではない。
// 真の値は常にアーカイブ内の level.dat から取る。
func TestParseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantVersion string
		wantLevel   string
		wantTime    bool
	}{
		{
			name:        "現行の形式",
			input:       "backup-26.2-world-20260910-143000.zip",
			wantVersion: "26.2", wantLevel: "world", wantTime: true,
		},
		{
			name:        "メモつき",
			input:       "backup-26.2-creative-20260910-143000-before-upgrade.zip",
			wantVersion: "26.2", wantLevel: "creative", wantTime: true,
		},
		{
			name:        "旧形式（ワールド名なし）",
			input:       "backup-26.2-20260901-003000.zip",
			wantVersion: "26.2", wantLevel: "", wantTime: true,
		},
		{
			name:        "人が付けた名前",
			input:       "my-backup.zip",
			wantVersion: "", wantLevel: "", wantTime: false,
		},
		{
			name:        "接頭辞だけ合っている",
			input:       "backup-.zip",
			wantVersion: "", wantLevel: "", wantTime: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := backup.ParseName(tt.input)

			if got.Version != tt.wantVersion {
				t.Errorf("Version が %q。%q のはず", got.Version, tt.wantVersion)
			}
			if got.Level != tt.wantLevel {
				t.Errorf("Level が %q。%q のはず", got.Level, tt.wantLevel)
			}
			if got.CreatedAt.IsZero() == tt.wantTime {
				t.Errorf("CreatedAt が %v（解釈できた=%v）", got.CreatedAt, !got.CreatedAt.IsZero())
			}
		})
	}
}

// 組み立てと解釈が往復する。
func TestNameRoundTrip(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 10, 14, 30, 0, 0, time.Local)
	name := backup.BuildName("26.2", "world", "", at)

	got := backup.ParseName(name)
	if got.Version != "26.2" || got.Level != "world" {
		t.Errorf("往復で失われた: %+v", got)
	}
	if !got.CreatedAt.Equal(at) {
		t.Errorf("日時が %v。%v のはず", got.CreatedAt, at)
	}
}

// メモがファイル名に使える形になるか。
// 日本語だけのメモは空になる。呼び出し側がそれを利用者に伝えられるよう、
// 「落ちた」ことが判別できる必要がある。
func TestNoteSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		note string
		want string
	}{
		{name: "空", note: "", want: ""},
		{name: "英数字はそのまま", note: "v2", want: "v2"},
		{name: "空白はハイフンに", note: "before upgrade", want: "before-upgrade"},
		{name: "日本語は丸ごと落ちる", note: "アップグレード前", want: ""},
		{name: "日本語混じりは英数字だけ残る", note: "v2 アップグレード前", want: "v2"},
		{name: "前後のハイフンは落とす", note: "  test  ", want: "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := backup.NoteSlug(tt.note); got != tt.want {
				t.Errorf("NoteSlug(%q) = %q。%q のはず", tt.note, got, tt.want)
			}
		})
	}
}
