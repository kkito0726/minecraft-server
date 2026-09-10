package backupctl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

const archiveID = "backup-26.2-world-20260901-000000.zip"

// --- 事前確認 ---

// 事前確認は副作用を一切持たない。
//
// 画面が「復元してよいか」を尋ねる前に呼ばれるため、
// ここでサーバーを止めたりファイルを触ったりしてはならない。
func TestPreflightHasNoSideEffects(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())

	if _, err := h.uc.PreflightRestore(context.Background(), archiveID); err != nil {
		t.Fatal(err)
	}
	if got := h.calls(); got != "" {
		t.Errorf("副作用がある: %q", got)
	}
}

func TestPreflight(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())

	got, err := h.uc.PreflightRestore(context.Background(), archiveID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Backup.ID.String() != archiveID {
		t.Errorf("ID が %q", got.Backup.ID)
	}
	if got.CurrentLevel.String() != "world" {
		t.Errorf("CurrentLevel が %q", got.CurrentLevel)
	}
	if got.ConfiguredVersion != "26.2" {
		t.Errorf("ConfiguredVersion が %q", got.ConfiguredVersion)
	}
	if got.RequiredBytes <= 0 {
		t.Errorf("RequiredBytes が %d", got.RequiredBytes)
	}
	if got.AvailableBytes <= 0 {
		t.Errorf("AvailableBytes が %d", got.AvailableBytes)
	}
}

// 展開後のサイズより多くの空きを要求する。
// ぴったりで始めると、途中で埋まって中断した状態が残る。
func TestPreflightRequiresMarginOverArchiveSize(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.info.TotalBytes = 1000
	h.store.seed(t, archiveID, h.now())

	got, err := h.uc.PreflightRestore(context.Background(), archiveID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiredBytes <= 1000 {
		t.Errorf("RequiredBytes が %d。展開後のサイズより大きいはず", got.RequiredBytes)
	}
}

// 存在しないバックアップは事前確認の時点で断る。
func TestPreflightMissing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.inspectErr = backup.ErrNotFound

	_, err := h.uc.PreflightRestore(context.Background(), archiveID)
	if !errors.Is(err, backup.ErrNotFound) {
		t.Errorf("エラーが %v。ErrNotFound のはず", err)
	}
}

func TestPreflightRejectsInvalidID(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	if _, err := h.uc.PreflightRestore(context.Background(), "../.env"); !errors.Is(err, backup.ErrInvalidID) {
		t.Errorf("エラーが %v。ErrInvalidID のはず", err)
	}
}

// --- 復元の実行 ---

// 復元は「停止 → 退避 → 展開 → 起動」の順。
//
// 退避が展開より先であることが最重要。逆順だと、バックアップに含まれない
// 新しい region ファイルが残り、古い地形と新しい地形が同居した
// 壊れたワールドになる。
func TestRestoreCallOrder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, Target: backupctl.TargetArchiveLevel, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "Down,WaitStopped,Quarantine,Extract,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
}

// 退避先は操作の属性に残す。自動では消さないので、
// 画面が「問題なく動いたら消してください」と案内できる必要がある。
func TestRestoreRecordsQuarantinePath(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.Attributes["quarantine_path"] == "" {
		t.Errorf("退避先が記録されていない: %v", snap.Attributes)
	}
	if !strings.Contains(snap.Attributes["quarantine_path"], ".broken-") {
		t.Errorf("退避先が %q", snap.Attributes["quarantine_path"])
	}
}

// 展開に失敗したら、退避したワールドを元に戻してからサーバーを起こす。
//
// これが漏れると、ワールドが .broken- のまま消え、利用者から見ると
// 「復元を試したらワールドが無くなった」ことになる。
func TestRestoreRollsBackOnExtractFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.store.extractErr = errArchive
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateFailed {
		t.Fatalf("状態が %v。失敗のはず", snap.State)
	}

	want := "Down,WaitStopped,Quarantine,Extract,Remove,RestoreQuarantine,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
	if !h.worlds.has("world") {
		t.Error("ワールドが戻っていない")
	}
}

// 巻き戻しは、展開の途中まで書かれたものを消してから戻す。
// 消さずに戻すと、展開されたファイルと元のファイルが混ざる。
func TestRestoreRemovesPartialBeforeRollback(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.store.extractErr = errArchive
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	calls := h.calls()
	remove := strings.Index(calls, "Remove,")
	restore := strings.Index(calls, "RestoreQuarantine")
	if remove < 0 || restore < 0 || remove > restore {
		t.Errorf("呼び出し順が %q。Remove が RestoreQuarantine より前のはず", calls)
	}
}

// 起動に失敗しても巻き戻さない。
//
// 展開そのものは終わっているので、戻すと復元した内容が失われる。
// 退避を残したまま失敗として報告し、人の判断に委ねる。
func TestRestoreDoesNotRollBackOnStartFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.runtime.upErr = errArchive
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateFailed {
		t.Fatalf("状態が %v。失敗のはず", snap.State)
	}

	if strings.Contains(h.calls(), "RestoreQuarantine") {
		t.Errorf("起動の失敗で巻き戻している: %q", h.calls())
	}
	if snap.Attributes["quarantine_path"] == "" {
		t.Error("退避先が残っていない。手で復旧できなくなる")
	}
}

// 復元先のワールドが無ければ退避はしない。退避するものが無い。
func TestRestoreSkipsQuarantineWhenTargetMissing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if strings.Contains(h.calls(), "Quarantine") {
		t.Errorf("退避するものが無いのに退避している: %q", h.calls())
	}
}

// --- 復元先の指定 ---

// 既定はアーカイブのワールド名。MC_LEVEL もそちらへ切り替える。
func TestRestoreToArchiveLevelSwitchesConfig(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"MC_LEVEL": "creative", "MC_VERSION": "26.2"})
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("creative", "world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, Target: backupctl.TargetArchiveLevel,
		AcknowledgeVersionWarning: true, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.config.get("MC_LEVEL"); got != "world" {
		t.Errorf("MC_LEVEL が %q。world のはず", got)
	}
	if !strings.Contains(h.calls(), "SaveConfig") {
		t.Errorf("設定を書き換えていない: %q", h.calls())
	}
}

// 稼働中のワールド名として復元する場合、展開時にパスを書き換える。
// MC_LEVEL は変えない。
func TestRestoreToCurrentLevelRewritesPath(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"MC_LEVEL": "creative", "MC_VERSION": "26.2"})
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("creative")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, Target: backupctl.TargetCurrentLevel,
		AcknowledgeVersionWarning: true, ConfirmLevelName: "creative",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.store.rewrites(); len(got) != 1 || got[0] != "creative" {
		t.Errorf("書き換え先が %v。creative のはず", got)
	}
	if h.config.get("MC_LEVEL") != "creative" {
		t.Errorf("MC_LEVEL が %q。変えないはず", h.config.get("MC_LEVEL"))
	}
	if strings.Contains(h.calls(), "SaveConfig") {
		t.Errorf("変える必要が無いのに設定を書き換えている: %q", h.calls())
	}
}

// 同じワールド名へ戻すだけなら設定は触らない。
func TestRestoreDoesNotTouchConfigWhenLevelUnchanged(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if strings.Contains(h.calls(), "SaveConfig") {
		t.Errorf("設定を触っている: %q", h.calls())
	}
}

// アーカイブのワールド名を判定できないとき、既定の復元先は決められない。
func TestRestoreRejectsUnknownArchiveLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.info.Level = ""
	h.store.seed(t, archiveID, h.now())

	_, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, Target: backupctl.TargetArchiveLevel,
		AcknowledgeVersionWarning: true, ConfirmLevelName: "world",
	})
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	if got := h.calls(); got != "" {
		t.Errorf("判定できないのに何かしている: %q", got)
	}
}

// --- 承諾のガード ---

// 承諾が要るのに承諾されていなければ、何もせず断る。
//
// サーバー側で必ず再判定する。画面が出した確認の結果は信用しない。
func TestRestoreRequiresAcknowledgement(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"MC_LEVEL": "creative", "MC_VERSION": "26.2"})
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("creative", "world")

	_, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if !errors.Is(err, backupctl.ErrConfirmationRequired) {
		t.Fatalf("エラーが %v。ErrConfirmationRequired のはず", err)
	}
	if got := h.calls(); got != "" {
		t.Errorf("断ったのに何かしている: %q", got)
	}
}

// 復元先の名前を打ち間違えていたら断る。
func TestRestoreRequiresMatchingConfirmName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		confirm string
	}{
		{name: "空", confirm: ""},
		{name: "打ち間違い", confirm: "wrold"},
		{name: "大文字小文字違い", confirm: "World"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, true, nil)
			h.store.seed(t, archiveID, h.now())
			h.worlds.add("world")

			_, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
				BackupID: archiveID, ConfirmLevelName: tt.confirm,
			})
			if !errors.Is(err, world.ErrConfirmationMismatch) {
				t.Errorf("エラーが %v。ErrConfirmationMismatch のはず", err)
			}
			if got := h.calls(); got != "" {
				t.Errorf("断ったのに何かしている: %q", got)
			}
		})
	}
}

// 空き容量が足りなければ、サーバーを止める前に断る。
func TestRestoreRejectsWhenDiskIsFull(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("world")
	h.worlds.available = 1

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateFailed {
		t.Fatalf("状態が %v。失敗のはず", snap.State)
	}
	if !strings.Contains(snap.ErrorMessage, "空き容量") {
		t.Errorf("エラーが %q。空き容量の不足だと分かるはず", snap.ErrorMessage)
	}
	if strings.Contains(h.calls(), "Down") {
		t.Errorf("容量不足なのにサーバーを止めている: %q", h.calls())
	}
}

// 停止中のサーバーは復元後も止めたままにする。
func TestRestoreKeepsServerStoppedWhenAlreadyStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.runtime.status.State = stoppedState()
	h.store.seed(t, archiveID, h.now())
	h.worlds.add("world")

	handle, err := h.uc.Restore(context.Background(), backupctl.RestoreRequest{
		BackupID: archiveID, ConfirmLevelName: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.calls(); got != "Quarantine,Extract" {
		t.Errorf("呼び出し順が %q。停止も起動もしないはず", got)
	}
}
