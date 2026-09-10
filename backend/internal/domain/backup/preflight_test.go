package backup_test

import (
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// 復元してよいかの判断。
//
// 「一致しているとき以外は必ず承諾を求める」が原則。ワールドの
// アップグレードは片道で、間違えると元のバージョンでは二度と開けない。
func TestDecideRestore(t *testing.T) {
	t.Parallel()

	same := version(t, 4903)
	older := version(t, 4800)
	newer := version(t, 5000)
	unreadable := shared.UnreadableWorldVersion()

	tests := []struct {
		name            string
		archive         shared.WorldVersion
		current         shared.WorldVersion
		archiveLevel    string
		currentLevel    string
		wantVerdict     backup.Verdict
		wantMismatch    bool
		wantConfirm     bool
		wantWarningWord string
	}{
		{
			name:    "一致していて名前も同じなら承諾は要らない",
			archive: same, current: same,
			archiveLevel: "world", currentLevel: "world",
			wantVerdict: backup.VerdictMatch,
			wantConfirm: false,
		},
		{
			name:    "アーカイブの方が古いとアップグレードが再走する",
			archive: older, current: same,
			archiveLevel: "world", currentLevel: "world",
			wantVerdict: backup.VerdictOlderWillUpgrade,
			wantConfirm: true, wantWarningWord: "片道",
		},
		{
			name:    "アーカイブの方が新しいと開けない可能性が高い",
			archive: newer, current: same,
			archiveLevel: "world", currentLevel: "world",
			wantVerdict: backup.VerdictNewerIncompatible,
			wantConfirm: true, wantWarningWord: "開けない",
		},
		{
			name:    "アーカイブのバージョンを読めない",
			archive: unreadable, current: same,
			archiveLevel: "world", currentLevel: "world",
			wantVerdict: backup.VerdictUnknown,
			wantConfirm: true, wantWarningWord: "読み取れ",
		},
		{
			name:    "現在のワールドのバージョンを読めない",
			archive: same, current: unreadable,
			archiveLevel: "world", currentLevel: "world",
			wantVerdict: backup.VerdictUnknown,
			wantConfirm: true, wantWarningWord: "読み取れ",
		},
		{
			name:    "ワールド名が食い違う",
			archive: same, current: same,
			archiveLevel: "world", currentLevel: "creative",
			wantVerdict:  backup.VerdictMatch,
			wantMismatch: true, wantConfirm: true, wantWarningWord: "creative",
		},
		{
			name:    "アーカイブのワールド名を判定できない",
			archive: same, current: same,
			archiveLevel: "", currentLevel: "world",
			wantVerdict:  backup.VerdictMatch,
			wantMismatch: true, wantConfirm: true, wantWarningWord: "判定できません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := backup.DecideRestore(tt.archive, tt.current, tt.archiveLevel, tt.currentLevel)

			if got.Verdict != tt.wantVerdict {
				t.Errorf("Verdict が %v。%v のはず", got.Verdict, tt.wantVerdict)
			}
			if got.LevelNameMismatch != tt.wantMismatch {
				t.Errorf("LevelNameMismatch が %v", got.LevelNameMismatch)
			}
			if got.RequiresConfirmation != tt.wantConfirm {
				t.Errorf("RequiresConfirmation が %v。警告: %v", got.RequiresConfirmation, got.Warnings)
			}
			if tt.wantWarningWord == "" {
				return
			}
			if !containsWord(got.Warnings, tt.wantWarningWord) {
				t.Errorf("警告が %v。%q を含むはず", got.Warnings, tt.wantWarningWord)
			}
		})
	}
}

// 承諾が要らないのは「一致していて名前も同じ」ときだけ。
//
// この条件を緩めると、警告を出すべき復元が黙って通る。
func TestDecideRestoreRequiresConfirmationUnlessFullyMatching(t *testing.T) {
	t.Parallel()

	same := version(t, 4903)
	got := backup.DecideRestore(same, same, "world", "world")

	if got.RequiresConfirmation {
		t.Error("完全に一致しているのに承諾を求めている")
	}
	if len(got.Warnings) != 0 {
		t.Errorf("警告が %v。無いはず", got.Warnings)
	}
}

// 承諾が必要なときは、理由が必ず 1 つ以上示される。
// 「確認してください」だけでは利用者は何を確認すべきか分からない。
func TestDecideRestoreAlwaysExplainsWhy(t *testing.T) {
	t.Parallel()

	same := version(t, 4903)
	older := version(t, 4800)

	cases := []backup.RestoreDecision{
		backup.DecideRestore(older, same, "world", "world"),
		backup.DecideRestore(same, same, "world", "creative"),
		backup.DecideRestore(shared.UnreadableWorldVersion(), same, "world", "world"),
	}
	for i, got := range cases {
		if !got.RequiresConfirmation {
			t.Fatalf("%d: 承諾が要らないことになっている", i)
		}
		if len(got.Warnings) == 0 {
			t.Errorf("%d: 理由が示されていない", i)
		}
	}
}

func containsWord(warnings []string, word string) bool {
	for _, w := range warnings {
		if strings.Contains(w, word) {
			return true
		}
	}
	return false
}
