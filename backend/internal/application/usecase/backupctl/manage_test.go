package backupctl_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// 一覧は新しい順に並ぶ。保管先の返す順序には依存しない。
func TestListSortsNewestFirst(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	base := time.Now().Add(-72 * time.Hour)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", base)
	h.store.seed(t, "backup-26.2-world-20260903-000000.zip", base.Add(48*time.Hour))
	h.store.seed(t, "backup-26.2-world-20260902-000000.zip", base.Add(24*time.Hour))

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backups) != 3 {
		t.Fatalf("%d 件", len(got.Backups))
	}

	want := []string{
		"backup-26.2-world-20260903-000000.zip",
		"backup-26.2-world-20260902-000000.zip",
		"backup-26.2-world-20260901-000000.zip",
	}
	for i, b := range got.Backups {
		if b.ID.String() != want[i] {
			t.Errorf("%d 番目が %q。%q のはず", i, b.ID, want[i])
		}
	}
	if got.TotalSizeBytes != 3*1024 {
		t.Errorf("合計が %d バイト", got.TotalSizeBytes)
	}
	if got.Directory == "" {
		t.Error("保管先が空")
	}
}

// 一覧はファイル名からバージョンとワールド名を読み取る。
// これは参考値であって、真の値はアーカイブ内の level.dat から取る。
func TestListReadsDeclaredVersionFromName(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "backup-26.2-creative-20260901-000000.zip", time.Now())

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Backups[0].DeclaredVersion != "26.2" {
		t.Errorf("DeclaredVersion が %q", got.Backups[0].DeclaredVersion)
	}
	if got.Backups[0].DeclaredLevel != "creative" {
		t.Errorf("DeclaredLevel が %q", got.Backups[0].DeclaredLevel)
	}
}

// 人が付けた名前のアーカイブも一覧に出す。読み取れない項目は空にする。
//
// 1 つの解釈できないファイルで一覧が空になってはならない。
func TestListKeepsUnparseableNames(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "my-backup.zip", time.Now())

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backups) != 1 {
		t.Fatalf("%d 件。読み飛ばしてはならない", len(got.Backups))
	}
	if got.Backups[0].DeclaredVersion != "" {
		t.Errorf("DeclaredVersion が %q。空のはず", got.Backups[0].DeclaredVersion)
	}
}

// 0 件でもエラーにならない。
func TestListEmpty(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backups) != 0 {
		t.Errorf("%d 件", len(got.Backups))
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", time.Now())

	freed, err := h.uc.Delete(context.Background(), "backup-26.2-world-20260901-000000.zip")
	if err != nil {
		t.Fatal(err)
	}
	if freed != 1024 {
		t.Errorf("解放が %d バイト", freed)
	}
	if len(h.store.names()) != 0 {
		t.Error("削除されていない")
	}
}

// ディレクトリ区切りを含む名前は保管先の外を指しうる。ID の生成時点で弾く。
func TestDeleteRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	for _, name := range []string{"../.env", "sub/backup.zip", "", "backup.txt"} {
		if _, err := h.uc.Delete(context.Background(), name); !errors.Is(err, backup.ErrInvalidID) {
			t.Errorf("%q のエラーが %v。ErrInvalidID のはず", name, err)
		}
	}
}

func TestDeleteMissing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	_, err := h.uc.Delete(context.Background(), "backup-26.2-world-20260901-000000.zip")
	if !errors.Is(err, backup.ErrNotFound) {
		t.Errorf("エラーが %v。ErrNotFound のはず", err)
	}
}

// 保持ポリシーは .env から読む。未設定なら既定値。
func TestGetRetentionPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    map[string]string
		wantCount int
		wantDays  int
	}{
		{
			name:      "未設定なら既定値",
			values:    map[string]string{},
			wantCount: 10, wantDays: 0,
		},
		{
			name:      "設定を読む",
			values:    map[string]string{"ADMIN_BACKUP_KEEP": "5", "ADMIN_BACKUP_KEEP_DAYS": "7"},
			wantCount: 5, wantDays: 7,
		},
		{
			name:      "壊れた値は既定値に倒す",
			values:    map[string]string{"ADMIN_BACKUP_KEEP": "abc", "ADMIN_BACKUP_KEEP_DAYS": "-3"},
			wantCount: 10, wantDays: 0,
		},
		{
			name:      "0 は既定値に倒す",
			values:    map[string]string{"ADMIN_BACKUP_KEEP": "0"},
			wantCount: 10, wantDays: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, true, tt.values)

			got, err := h.uc.GetRetentionPolicy(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.KeepCount() != tt.wantCount || got.KeepDays() != tt.wantDays {
				t.Errorf("ポリシーが %d/%d。%d/%d のはず",
					got.KeepCount(), got.KeepDays(), tt.wantCount, tt.wantDays)
			}
		})
	}
}

// 保持ポリシーは .env へ書き戻す。
func TestSetRetentionPolicy(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	got, err := h.uc.SetRetentionPolicy(context.Background(), 3, 14)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeepCount() != 3 || got.KeepDays() != 14 {
		t.Errorf("ポリシーが %d/%d", got.KeepCount(), got.KeepDays())
	}
	if h.config.get("ADMIN_BACKUP_KEEP") != "3" {
		t.Errorf("ADMIN_BACKUP_KEEP が %q", h.config.get("ADMIN_BACKUP_KEEP"))
	}
	if h.config.get("ADMIN_BACKUP_KEEP_DAYS") != "14" {
		t.Errorf("ADMIN_BACKUP_KEEP_DAYS が %q", h.config.get("ADMIN_BACKUP_KEEP_DAYS"))
	}
}

// TC-006-B02: 保持世代数の下限。0 以下は .env に書く前に断る。
func TestSetRetentionPolicyRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
		days  int
	}{
		{name: "0 世代", count: 0, days: 0},
		{name: "負の世代数", count: -1, days: 0},
		{name: "負の日数", count: 10, days: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, true, nil)

			_, err := h.uc.SetRetentionPolicy(context.Background(), tt.count, tt.days)
			if !errors.Is(err, backup.ErrInvalidRetentionPolicy) {
				t.Errorf("エラーが %v。ErrInvalidRetentionPolicy のはず", err)
			}
			if strings.Contains(h.calls(), "SaveConfig") {
				t.Error("不正な値なのに .env へ書き込んでいる")
			}
		})
	}
}

// TC-006-01: 世代数の枠から溢れたものを削除する。
func TestPrune(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"ADMIN_BACKUP_KEEP": "2"})
	base := time.Now().Add(-240 * time.Hour)
	for i, name := range []string{
		"backup-26.2-world-20260901-000000.zip",
		"backup-26.2-world-20260902-000000.zip",
		"backup-26.2-world-20260903-000000.zip",
		"backup-26.2-world-20260904-000000.zip",
	} {
		h.store.seed(t, name, base.Add(time.Duration(i)*24*time.Hour))
	}

	got, err := h.uc.Prune(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DeletedIDs) != 2 {
		t.Fatalf("削除が %d 件: %v", len(got.DeletedIDs), got.DeletedIDs)
	}
	if got.FreedBytes != 2*1024 {
		t.Errorf("解放が %d バイト", got.FreedBytes)
	}
	if len(h.store.names()) != 2 {
		t.Errorf("残りが %d 件", len(h.store.names()))
	}
}

// dry_run は対象を返すだけで削除しない。
func TestPruneDryRun(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"ADMIN_BACKUP_KEEP": "1"})
	base := time.Now().Add(-240 * time.Hour)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", base)
	h.store.seed(t, "backup-26.2-world-20260902-000000.zip", base.Add(24*time.Hour))

	got, err := h.uc.Prune(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DeletedIDs) != 1 {
		t.Fatalf("対象が %d 件", len(got.DeletedIDs))
	}
	if len(h.store.names()) != 2 {
		t.Error("dry_run なのに削除している")
	}
	if strings.Contains(h.calls(), "DeleteArchive") {
		t.Errorf("dry_run なのに削除を呼んでいる: %q", h.calls())
	}
}

// TC-006-B01: 設定がどうであれ最も新しい 1 件は必ず残る。
func TestPruneAlwaysKeepsNewest(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{
		"ADMIN_BACKUP_KEEP": "1", "ADMIN_BACKUP_KEEP_DAYS": "1",
	})
	oneYearAgo := time.Now().Add(-365 * 24 * time.Hour)
	h.store.seed(t, "backup-26.2-world-20250901-000000.zip", oneYearAgo)
	h.store.seed(t, "backup-26.2-world-20250902-000000.zip", oneYearAgo.Add(time.Hour))

	if _, err := h.uc.Prune(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := h.store.names(); len(got) != 1 {
		t.Errorf("残りが %d 件。1 件は必ず残るはず: %v", len(got), got)
	}
}

// TC-006-B03: 0 件でもエラーにならない。
func TestPruneEmpty(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	got, err := h.uc.Prune(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DeletedIDs) != 0 {
		t.Errorf("削除が %d 件", len(got.DeletedIDs))
	}
}

// 削除の判定にはファイルの更新時刻を使う。
// ファイル名の日時は人が改名できるため、保持の判断材料にしない。
func TestPruneUsesFileTimeNotName(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"ADMIN_BACKUP_KEEP": "1"})
	// 名前は古いが、ファイルとしては新しい
	h.store.seed(t, "backup-26.2-world-20200101-000000.zip", time.Now())
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", time.Now().Add(-240*time.Hour))

	got, err := h.uc.Prune(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DeletedIDs) != 1 || got.DeletedIDs[0].String() != "backup-26.2-world-20260901-000000.zip" {
		t.Errorf("削除対象が %v。更新時刻の古い方のはず", got.DeletedIDs)
	}
}

// 一覧はアーカイブ内の level.dat から真のバージョンを読む。
// ファイル名は人が改名できるため、名前と中身が食い違いうる。
func TestListReadsVersionFromArchive(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", time.Now())

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Backups[0].ArchiveLevel != "world" {
		t.Errorf("ArchiveLevel が %q", got.Backups[0].ArchiveLevel)
	}
	if len(got.Backups[0].EntryRoots) == 0 {
		t.Error("EntryRoots が空")
	}
}

// EDGE-001: アーカイブを読めなくても一覧からは消さない。
//
// 読めないものを隠すと、画面から削除する手段まで失われる。
func TestListKeepsUnreadableArchive(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.inspectErr = errArchive
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", time.Now())

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backups) != 1 {
		t.Fatalf("%d 件。読めなくても一覧には出すはず", len(got.Backups))
	}
	if got.Backups[0].Version.Readable() {
		t.Error("読めないはずのバージョンが読めたことになっている")
	}
	if got.Backups[0].ID.String() != "backup-26.2-world-20260901-000000.zip" {
		t.Errorf("ID が %q", got.Backups[0].ID)
	}
}

// level.dat を含まないアーカイブでも一覧は成立する。
func TestListWithoutLevelDat(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.info.HasLevelDat = false
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", time.Now())

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Backups[0].Version.Readable() {
		t.Error("level.dat が無いのにバージョンが読めたことになっている")
	}
}
