package worldctl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	if _, err := worldctl.New(worldctl.Config{}); err == nil {
		t.Error("依存が不足していればエラーになるはず")
	}
}

// .env に不正な MC_LEVEL が手で書かれていても、一覧は返す。
// 画面が全く開けなくなるより、稼働中の印が付かない方がまし。
func TestListToleratesInvalidActiveLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.config.mu.Lock()
	h.config.values["MC_LEVEL"] = "../etc"
	h.config.mu.Unlock()

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatalf("一覧は返すはず: %v", err)
	}
	if len(got.Worlds) == 0 {
		t.Error("ワールドが返っていない")
	}
	if got.ActiveLevel.IsValid() {
		t.Error("不正な名前が有効な Name として返っている")
	}
}

// 新規作成で起動に失敗したら、操作は失敗として記録される。
func TestCreateFailurePropagates(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.runtime.upErr = errors.New("起動に失敗")

	handle, err := h.uc.Create(context.Background(), "fresh", "")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
	if !strings.Contains(snap.ErrorMessage, "起動に失敗") {
		t.Errorf("失敗理由が %q", snap.ErrorMessage)
	}
}

// 改名で起動に失敗した場合も同様。
func TestRenameActiveFailurePropagates(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.runtime.upErr = errors.New("起動に失敗")

	handle, err := h.uc.Rename(context.Background(), "world", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
}

func TestRenameRejectsMissingSource(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	if _, err := h.uc.Rename(context.Background(), "nope", "new"); err == nil {
		t.Error("存在しない改名元は拒否されるはず")
	}
}

func TestRenameRejectsExistingDestination(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")
	if _, err := h.uc.Rename(context.Background(), "creative", "world"); err == nil {
		t.Error("既存の名前への改名は拒否されるはず")
	}
}

func TestDeleteRejectsMissingWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	if _, err := h.uc.Delete(context.Background(), "nope", "nope", true); err == nil {
		t.Error("存在しないワールドの削除は拒否されるはず")
	}
}

// 保存に失敗しても切替は続行する。
// 停止できないより、直近の数秒が失われる方がまだよい
// （stop_grace_period 中にサーバー自身も保存する）。
func TestSwitchContinuesWhenSaveFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "creative")
	h.console().saveAllErr = errors.New("保存に失敗")

	handle, err := h.uc.Switch(context.Background(), "creative")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateSucceeded {
		t.Errorf("保存の失敗で切替を止めないはず: %v (%s)", snap.State, snap.ErrorMessage)
	}
	if h.config.get("MC_LEVEL") != "creative" {
		t.Error("切替が完了していない")
	}
}

// 複製で保存の停止に失敗したら、複製自体を行わない。
// 書き込み中の region ファイルを掴んで壊れたコピーを作るより安全。
func TestCloneAbortsWhenSaveOffFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.console().saveAllErr = errors.New("save-all に失敗")

	handle, err := h.uc.Clone(context.Background(), "world", "backup")
	if err != nil {
		t.Fatal(err)
	}
	snap := h.wait(t, handle.ID())

	if snap.State != operation.StateFailed {
		t.Errorf("State が %v", snap.State)
	}
	if strings.Contains(h.calls(), "Copy") {
		t.Errorf("保存に失敗したのに複製している: %q", h.calls())
	}
	// それでも save-on は戻す
	if !strings.Contains(h.calls(), "SaveOn") {
		t.Errorf("save-on が呼ばれていない: %q", h.calls())
	}
}
