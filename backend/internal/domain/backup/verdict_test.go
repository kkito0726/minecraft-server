package backup_test

import (
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

func version(t *testing.T, dv int32) shared.WorldVersion {
	t.Helper()

	v, err := shared.NewDataVersion(dv)
	if err != nil {
		t.Fatal(err)
	}
	return shared.NewWorldVersion("26.2", v, false, "world")
}

// ワールドのアップグレードは片道。復元前にバージョンの食い違いを
// 検出できないと、戻せない変更を気づかずに走らせてしまう。
func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		archive shared.WorldVersion
		current shared.WorldVersion
		want    backup.Verdict
	}{
		{
			name:    "一致",
			archive: version(t, 4903),
			current: version(t, 4903),
			want:    backup.VerdictMatch,
		},
		{
			name:    "アーカイブの方が古い",
			archive: version(t, 4820),
			current: version(t, 4903),
			want:    backup.VerdictOlderWillUpgrade,
		},
		{
			name:    "アーカイブの方が新しい",
			archive: version(t, 4950),
			current: version(t, 4903),
			want:    backup.VerdictNewerIncompatible,
		},
		{
			name:    "アーカイブが読めない",
			archive: shared.UnreadableWorldVersion(),
			current: version(t, 4903),
			want:    backup.VerdictUnknown,
		},
		{
			name:    "現在のワールドが読めない",
			archive: version(t, 4903),
			current: shared.UnreadableWorldVersion(),
			want:    backup.VerdictUnknown,
		},
		{
			name:    "どちらも読めない",
			archive: shared.UnreadableWorldVersion(),
			current: shared.UnreadableWorldVersion(),
			want:    backup.VerdictUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := backup.CompareVersions(tt.archive, tt.current); got != tt.want {
				t.Errorf("CompareVersions() = %v。%v のはず", got, tt.want)
			}
		})
	}
}

// 表示文字列が同じでも DataVersion が違えば一致としない。
// ここを取り違えると、見た目が同じバージョンのワールドを上書きしてしまう。
func TestCompareVersionsIgnoresDisplayName(t *testing.T) {
	t.Parallel()

	dvOld, _ := shared.NewDataVersion(4820)
	dvNew, _ := shared.NewDataVersion(4903)

	// どちらも表示は "26.2" だが中身が違う
	archive := shared.NewWorldVersion("26.2", dvOld, false, "world")
	current := shared.NewWorldVersion("26.2", dvNew, false, "world")

	if got := backup.CompareVersions(archive, current); got != backup.VerdictOlderWillUpgrade {
		t.Errorf("表示文字列に引きずられて %v になった", got)
	}
}

// 一致以外は利用者の承諾を必要とする。
func TestVerdictRequiresConfirmation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		verdict backup.Verdict
		want    bool
	}{
		{backup.VerdictMatch, false},
		{backup.VerdictOlderWillUpgrade, true},
		{backup.VerdictNewerIncompatible, true},
		{backup.VerdictUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.verdict.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.verdict.RequiresConfirmation(); got != tt.want {
				t.Errorf("RequiresConfirmation() = %v。%v のはず", got, tt.want)
			}
		})
	}
}

// 警告文はそのまま画面に出す。何が起きるかが書かれていること。
func TestVerdictWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		verdict  backup.Verdict
		contains string
	}{
		{backup.VerdictOlderWillUpgrade, "アップグレード"},
		{backup.VerdictNewerIncompatible, "開けない"},
		{backup.VerdictUnknown, "確認できません"},
	}

	for _, tt := range tests {
		t.Run(tt.verdict.String(), func(t *testing.T) {
			t.Parallel()
			got := tt.verdict.Warning()
			if got == "" {
				t.Fatal("警告文が空")
			}
			if !strings.Contains(got, tt.contains) {
				t.Errorf("警告文 %q に %q が含まれていない", got, tt.contains)
			}
		})
	}

	if backup.VerdictMatch.Warning() != "" {
		t.Error("一致のときは警告を出さない")
	}
}
