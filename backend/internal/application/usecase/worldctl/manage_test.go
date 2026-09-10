package worldctl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

func TestList(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatalf("List に失敗: %v", err)
	}
	if len(got.Worlds) != 2 {
		t.Errorf("ワールドが %d 件", len(got.Worlds))
	}
	if got.ActiveLevel.String() != "world" {
		t.Errorf("ActiveLevel が %q", got.ActiveLevel)
	}
}

// 稼働中のワールドを複製するときは保存を止める。
// 書き込み途中の region ファイルを掴むと壊れたコピーになる。
func TestCloneStopsSavingWhileRunning(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Clone(context.Background(), "world", "backup")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "SaveOff,SaveAll,Copy,SaveOn"
	if got := h.calls(); !strings.Contains(got, want) {
		t.Errorf("呼び出し順が %q。%q を含むはず", got, want)
	}
	if !h.worlds.has("backup") {
		t.Error("複製先が作られていない")
	}
}

// 複製が失敗しても save-on は必ず呼ばれる。
//
// これが漏れると、以降の変更がディスクに書かれないのに症状が何も出ない。
// 次の再起動で数時間分のプレイが消えて初めて気づくことになる。
func TestCloneRestoresSavingOnFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.worlds.copyErr = errors.New("ディスクが一杯です")

	handle, err := h.uc.Clone(context.Background(), "world", "backup")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
	calls := h.calls()
	if !strings.Contains(calls, "SaveOn") {
		t.Errorf("失敗時に save-on が呼ばれていない: %q", calls)
	}
	// SaveOff より後に SaveOn があること
	if strings.Index(calls, "SaveOn") < strings.Index(calls, "SaveOff") {
		t.Errorf("save-on が save-off より前にある: %q", calls)
	}
}

// 停止中なら保存の停止も不要。
func TestCloneSkipsSaveWhenStopped(t *testing.T) {
	t.Parallel()

	h := newHarness(t, false, "world")

	handle, err := h.uc.Clone(context.Background(), "world", "backup")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if strings.Contains(h.calls(), "SaveOff") {
		t.Errorf("停止中なのに保存を止めている: %q", h.calls())
	}
}

// 複製は切り替えを伴わない。
func TestCloneDoesNotSwitch(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, _ := h.uc.Clone(context.Background(), "world", "backup")
	h.wait(t, handle.ID())

	if h.config.get("MC_LEVEL") != "world" {
		t.Errorf("MC_LEVEL が %q に変わっている", h.config.get("MC_LEVEL"))
	}
	if strings.Contains(h.calls(), "Up") {
		t.Errorf("複製で起動している: %q", h.calls())
	}
}

// 空き容量が足りなければ 1 バイトもコピーしない。
func TestCloneChecksSpaceFirst(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.worlds.available = 1 // 明らかに足りない

	handle, err := h.uc.Clone(context.Background(), "world", "backup")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
	if !strings.Contains(snap.ErrorMessage, "空き容量") {
		t.Errorf("失敗理由が %q", snap.ErrorMessage)
	}
	if strings.Contains(h.calls(), "Copy") {
		t.Errorf("容量不足なのに複製している: %q", h.calls())
	}
}

func TestCloneRejectsExistingDestination(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")
	if _, err := h.uc.Clone(context.Background(), "world", "creative"); err == nil {
		t.Error("既存の名前は拒否されるはず")
	}
}

func TestCloneRejectsMissingSource(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	if _, err := h.uc.Clone(context.Background(), "nope", "backup"); err == nil {
		t.Error("存在しない複製元は拒否されるはず")
	}
}

// 非稼働のワールドの改名は、停止を伴わない。
func TestRenameInactiveWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "old")

	handle, err := h.uc.Rename(context.Background(), "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.calls(); got != "Rename" {
		t.Errorf("呼び出しが %q。Rename だけのはず", got)
	}
	if !h.worlds.has("new") || h.worlds.has("old") {
		t.Error("改名されていない")
	}
}

// 稼働中のワールドの改名は、停止して MC_LEVEL も追随させる。
// 停止せずに改名すると、サーバーが開いているディレクトリが消える。
func TestRenameActiveWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Rename(context.Background(), "world", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "SaveAll,Down,WaitStopped,Rename,SaveConfig,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
	if h.config.get("MC_LEVEL") != "renamed" {
		t.Errorf("MC_LEVEL が %q", h.config.get("MC_LEVEL"))
	}
}

// 稼働中のワールドは削除できない。消すと復旧できない。
func TestDeleteRejectsActiveWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	_, err := h.uc.Delete(context.Background(), "world", "world", true)
	if !errors.Is(err, world.ErrActiveWorld) {
		t.Errorf("ErrActiveWorld を期待したが %v", err)
	}
}

// 確認の名前が一致しなければ拒否する。
func TestDeleteRequiresConfirmation(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	if _, err := h.uc.Delete(context.Background(), "creative", "typo", true); err == nil {
		t.Error("確認名が違えば拒否されるはず")
	}
	if len(h.rec.list()) != 0 {
		t.Errorf("拒否したのに副作用がある: %q", h.calls())
	}
}

// 既定は退避。即座には消さない。
func TestDeleteQuarantinesByDefault(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	handle, err := h.uc.Delete(context.Background(), "creative", "creative", true)
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	if got := h.calls(); got != "Quarantine" {
		t.Errorf("呼び出しが %q。Quarantine のはず", got)
	}
	if snap.Attributes["quarantine_path"] == "" {
		t.Error("退避先が属性に記録されていない")
	}
}

// 明示すれば即座に削除する。
func TestDeleteImmediately(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	handle, err := h.uc.Delete(context.Background(), "creative", "creative", false)
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if got := h.calls(); got != "Remove" {
		t.Errorf("呼び出しが %q。Remove のはず", got)
	}
}

// 退避したものを完全に削除できる。
func TestPurgeQuarantine(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	handle, err := h.uc.Delete(context.Background(), "creative", "creative", true)
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	dirName := snap.Attributes["quarantine_path"]

	freed, err := h.uc.PurgeQuarantine(context.Background(), dirName)
	if err != nil {
		t.Fatalf("PurgeQuarantine に失敗: %v", err)
	}
	if freed <= 0 {
		t.Errorf("解放したサイズが %d", freed)
	}

	list, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Quarantines) != 0 {
		t.Errorf("退避が %d 件残っている", len(list.Quarantines))
	}
}

func TestPurgeQuarantineNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	if _, err := h.uc.PurgeQuarantine(context.Background(), "nope.broken-20260101-000000"); err == nil {
		t.Error("エラーになるはず")
	}
}

// 新規作成はディレクトリを作らず、サーバーに生成させる。
func TestCreateDelegatesGenerationToServer(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Create(context.Background(), "fresh", "12345")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}

	want := "SaveAll,Down,WaitStopped,SaveConfig,Up,WaitReady"
	if got := h.calls(); got != want {
		t.Errorf("呼び出し順が %q。%q のはず", got, want)
	}
	if h.config.get("MC_LEVEL") != "fresh" {
		t.Errorf("MC_LEVEL が %q", h.config.get("MC_LEVEL"))
	}
	if h.config.get("MC_SEED") != "12345" {
		t.Errorf("MC_SEED が %q", h.config.get("MC_SEED"))
	}
}

func TestCreateRejectsExisting(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	if _, err := h.uc.Create(context.Background(), "world", ""); err == nil {
		t.Error("既存の名前は拒否されるはず")
	}
}

// 切替では MC_SEED を空に戻す。
//
// 新規生成の待ちは最大 900 秒に及び、その間にプロセスが停止すると
// シードが残る。残ったまま次のワールドを作ると同じ地形になる。
func TestSwitchClearsSeed(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")

	// 生成が中断してシードが残った状態を再現
	created, err := h.uc.Create(context.Background(), "seeded", "99999")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, created.ID())
	if h.config.get("MC_SEED") != "99999" {
		t.Fatal("前提が崩れている")
	}

	handle, err := h.uc.Switch(context.Background(), "creative")
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())

	if got := h.config.get("MC_SEED"); got != "" {
		t.Errorf("MC_SEED が %q。空に戻るはず", got)
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	ctx := context.Background()
	bad := "../etc"

	if _, err := h.uc.Create(ctx, bad, ""); err == nil {
		t.Error("Create が不正な名前を受理した")
	}
	if _, err := h.uc.Clone(ctx, bad, "dst"); err == nil {
		t.Error("Clone が不正な複製元を受理した")
	}
	if _, err := h.uc.Clone(ctx, "world", bad); err == nil {
		t.Error("Clone が不正な複製先を受理した")
	}
	if _, err := h.uc.Rename(ctx, bad, "dst"); err == nil {
		t.Error("Rename が不正な名前を受理した")
	}
	if _, err := h.uc.Delete(ctx, bad, bad, true); err == nil {
		t.Error("Delete が不正な名前を受理した")
	}
}
