package worldctl_test

import (
	"context"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
)

// MC_HARDCORE はサーバー全体の値で、ハードコアかどうかは本来ワールドが
// level.dat に持っている。切り替えのたびに、切り替え先に合わせる。

// switchTo は切り替えて終わるまで待つ。
func (h *harness) switchTo(t *testing.T, name string) {
	t.Helper()

	handle, err := h.uc.Switch(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("切り替えに失敗: %s", snap.ErrorMessage)
	}
}

// ハードコアのワールドから普通のワールドへ移ったら、ハードコアを外す。
// 外さないと、普通に作ったワールドで死亡が不可逆になる。
func TestSwitchLeavesHardcore(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "casual")
	h.config.values["MC_HARDCORE"] = "TRUE"
	h.config.values["MC_DIFFICULTY"] = "hard"
	h.levels.settings["casual"] = port.LevelSettings{
		HasHardcore: true, Hardcore: false,
		HasDifficulty: true, Difficulty: settings.DifficultyEasy,
	}

	h.switchTo(t, "casual")

	if got := h.config.get("MC_HARDCORE"); got != "FALSE" {
		t.Errorf("MC_HARDCORE が %q。外れるはず", got)
	}
	// ハードコアで固定されていた難易度は、そのワールドの難易度に戻す。
	if got := h.config.get("MC_DIFFICULTY"); got != "easy" {
		t.Errorf("MC_DIFFICULTY が %q。ワールドの easy に戻るはず", got)
	}
}

// 普通のワールドからハードコアのワールドへ戻ったら、ハードコアに戻す。
func TestSwitchEntersHardcore(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "hardmode")
	h.config.values["MC_HARDCORE"] = "FALSE"
	h.config.values["MC_DIFFICULTY"] = "peaceful"
	h.levels.settings["hardmode"] = port.LevelSettings{HasHardcore: true, Hardcore: true}

	h.switchTo(t, "hardmode")

	if got := h.config.get("MC_HARDCORE"); got != "TRUE" {
		t.Errorf("MC_HARDCORE が %q。戻るはず", got)
	}
	if got := h.config.get("MC_DIFFICULTY"); got != "hard" {
		t.Errorf("MC_DIFFICULTY が %q。ハードに固定されるはず", got)
	}
}

// level.dat を読めなければ触らない。「偽」と決めつけて FALSE を書くと、
// 読めなかっただけのハードコアのワールドを普通にしてしまう。
func TestSwitchKeepsHardcoreWhenUnreadable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "broken")
	h.config.values["MC_HARDCORE"] = "TRUE"
	// broken の設定は登録しない（読めない）

	h.switchTo(t, "broken")

	if got := h.config.get("MC_HARDCORE"); got != "TRUE" {
		t.Errorf("MC_HARDCORE が %q。触らないはず", got)
	}
}

// 普通のワールドどうしの切り替えでは、難易度に触らない。
// 難易度は /settings で決めるサーバー全体の設定で、切り替えのたびに
// 変わると驚かせる。
func TestSwitchBetweenNormalWorldsKeepsDifficulty(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "other")
	h.config.values["MC_HARDCORE"] = "FALSE"
	h.config.values["MC_DIFFICULTY"] = "peaceful"
	h.levels.settings["other"] = port.LevelSettings{
		HasHardcore: true, Hardcore: false,
		HasDifficulty: true, Difficulty: settings.DifficultyHard,
	}

	h.switchTo(t, "other")

	if got := h.config.get("MC_DIFFICULTY"); got != "peaceful" {
		t.Errorf("MC_DIFFICULTY が %q。触らないはず", got)
	}
}

// 一覧でハードコアのワールドが分かること。切り替える前に気づけるように。
func TestListMarksHardcoreWorlds(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "hardmode")
	h.levels.settings["hardmode"] = port.LevelSettings{HasHardcore: true, Hardcore: true}

	got, err := h.uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	marks := map[string]bool{}
	for _, w := range got.Worlds {
		marks[w.Name().String()] = w.IsHardcore()
	}
	if !marks["hardmode"] {
		t.Error("hardmode にハードコアの印が無い")
	}
	if marks["world"] {
		t.Error("world にハードコアの印が付いている")
	}
}
