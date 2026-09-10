package backupctl_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// TC-004-01: HOT バックアップは save-off → save-all → 取得 → save-on の順。
//
// docs/backup-restore.md の手順そのもの。save-all を省くと、
// メモリ上にしかないチャンクがアーカイブに入らない。
func TestCreateHotCallOrder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "SaveOff,SaveAll,CreateArchive,SaveOn"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
}

// TC-004-E01: アーカイブ作成が失敗しても save-on は必ず呼ばれる。
//
// これが破れると、以降の変更がディスクに書かれないのに症状が何も出ない。
// 次の再起動で数時間分のプレイが消えて初めて気づくことになる。
func TestCreateHotRestoresSavingOnFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.createErr = errArchive

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateFailed {
		t.Fatalf("状態が %v。失敗のはず", snap.State)
	}

	want := "SaveOff,SaveAll,CreateArchive,SaveOn"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
}

// save-off を送っている間はロックにその事実が記録される。
//
// プロセスが落ちても次の起動で「save-off が残っているかもしれない」と
// 分かるようにするため。記録は save-off の前に立て、save-on の後に降ろす。
// 逆順にすると、その隙間で落ちたときに「保存は正常」と誤って記録される。
func TestCreateHotRecordsSaveDisabled(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	history := h.lock.saveDisabledHistory()
	if !containsTrue(history) {
		t.Fatalf("SaveDisabled の記録が %v。途中で真になるはず", history)
	}
	if history[len(history)-1] {
		t.Errorf("SaveDisabled の記録が %v。最後は偽のはず", history)
	}
}

// 失敗した場合でも、save-on を送った後に記録を降ろす。
func TestCreateHotClearsSaveDisabledOnFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.createErr = errArchive

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	history := h.lock.saveDisabledHistory()
	if history[len(history)-1] {
		t.Errorf("SaveDisabled の記録が %v。最後は偽のはず", history)
	}
}

// TC-004-E03: サーバーが停止していれば RCON を呼ばない。
//
// 停止中は保存も走っていないため save-off は不要で、
// 呼んでも接続できずに失敗するだけになる。
func TestCreateHotSkipsConsoleWhenStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, false, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.calls(); got != "CreateArchive" {
		t.Errorf("呼び出し順が %q。RCON を呼ばないはず", got)
	}
}

// TC-004-06: COLD バックアップは停止 → 停止完了の確認 → 取得 → 起動 → 起動完了の確認。
//
// Down の完了を待たずに取ると、stop_grace_period（60 秒）の間に
// Paper が書いている region ファイルを掴む。COLD で取る意味が消える。
func TestCreateColdCallOrder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeCold, "")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "Down,WaitStopped,CreateArchive,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
	if strings.Contains(h.calls(), "Save") {
		t.Errorf("COLD で RCON を呼んでいる: %q", h.calls())
	}
}

// COLD で取得に失敗してもサーバーは起動し直す。
// 停止したまま放置すると、バックアップの失敗がサーバーの停止に化ける。
func TestCreateColdRestartsServerOnFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.createErr = errArchive

	handle, err := h.uc.Create(context.Background(), backupctl.ModeCold, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateFailed {
		t.Fatalf("状態が %v。失敗のはず", snap.State)
	}

	if got := h.calls(); !strings.Contains(got, "CreateArchive,Up") {
		t.Errorf("呼び出し順が %q。失敗後も起動するはず", got)
	}
}

// 停止中に COLD で取るときは、取得後に起動しない。
// 利用者が止めておいたサーバーを、バックアップが勝手に起動してはならない。
func TestCreateColdKeepsServerStoppedWhenAlreadyStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, false, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeCold, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.calls(); got != "CreateArchive" {
		t.Errorf("呼び出し順が %q。停止中は停止も起動もしないはず", got)
	}
}

// TC-004-05: 対象のワールドは MC_LEVEL に従う。world を固定値にしない。
func TestCreateUsesConfiguredLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"MC_LEVEL": "creative", "MC_VERSION": "26.2"})

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.store.levels(); len(got) != 1 || got[0] != "creative" {
		t.Errorf("対象のワールドが %v。creative のはず", got)
	}
	if names := h.store.names(); len(names) != 1 || !strings.Contains(names[0], "-creative-") {
		t.Errorf("ファイル名が %v。creative を含むはず", names)
	}
}

// MC_LEVEL が未設定なら compose.yaml の既定値 world を使う。
func TestCreateFallsBackToDefaultLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{"MC_VERSION": "26.2"})

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.store.levels(); len(got) != 1 || got[0] != "world" {
		t.Errorf("対象のワールドが %v。world のはず", got)
	}
}

// TC-004-03 / TC-004-04: ファイル名の形式。
func TestCreateFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		note     string
		contains string
	}{
		{name: "メモなし", note: "", contains: "backup-26.2-world-"},
		{name: "メモあり", note: "before upgrade", contains: "-before-upgrade.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, true, nil)

			handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, tt.note)
			if err != nil {
				t.Fatal(err)
			}
			if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
				t.Fatalf("失敗した: %s", snap.ErrorMessage)
			}

			names := h.store.names()
			if len(names) != 1 || !strings.Contains(names[0], tt.contains) {
				t.Errorf("ファイル名が %v。%q を含むはず", names, tt.contains)
			}
		})
	}
}

// 取得したファイル名は操作の属性に記録される。画面が結果を指し示せるようにする。
func TestCreateRecordsBackupIDAttribute(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	got := snap.Attributes["backup_id"]
	if !strings.HasPrefix(got, "backup-") {
		t.Errorf("backup_id が %q", got)
	}
}

// TC-006-01: 取得の成功直後に保持ポリシーを適用する。
func TestCreateAppliesRetention(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{
		"MC_LEVEL": "world", "MC_VERSION": "26.2", "ADMIN_BACKUP_KEEP": "2",
	})
	base := time.Now().Add(-72 * time.Hour)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", base)
	h.store.seed(t, "backup-26.2-world-20260902-000000.zip", base.Add(time.Hour))

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := len(h.store.names()); got != 2 {
		t.Errorf("バックアップが %d 件。2 件のはず: %v", got, h.store.names())
	}
}

// TC-006-04: 取得が失敗したときは保持ポリシーを適用しない。
//
// 適用してしまうと、新しいものが増えていないのに古いものだけ消える。
func TestCreateSkipsRetentionOnFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, map[string]string{
		"MC_LEVEL": "world", "MC_VERSION": "26.2", "ADMIN_BACKUP_KEEP": "1",
	})
	base := time.Now().Add(-72 * time.Hour)
	h.store.seed(t, "backup-26.2-world-20260901-000000.zip", base)
	h.store.seed(t, "backup-26.2-world-20260902-000000.zip", base.Add(time.Hour))
	h.store.createErr = errArchive

	handle, err := h.uc.Create(context.Background(), backupctl.ModeHot, "")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if got := len(h.store.names()); got != 2 {
		t.Errorf("バックアップが %d 件。1 件も消えないはず", got)
	}
	if strings.Contains(h.calls(), "DeleteArchive") {
		t.Errorf("失敗したのに削除している: %q", h.calls())
	}
}

// 未知のモードは受け付けない。
func TestCreateRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)

	if _, err := h.uc.Create(context.Background(), backupctl.Mode(99), ""); err == nil {
		t.Error("未知のモードが受理された")
	}
}

func containsTrue(values []bool) bool {
	for _, v := range values {
		if v {
			return true
		}
	}
	return false
}
