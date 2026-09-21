package worldctl_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// --- 一覧 ---

func TestListVersions(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	got, err := h.uc.ListVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Current != "26.2" {
		t.Errorf("一覧が %+v", got)
	}
	if !slices.Equal(got.Versions, h.catalog.versions) {
		t.Errorf("版が %v", got.Versions)
	}
}

// 一覧が取れなくても失敗にしない。現在の版で作ることまで塞がないため。
func TestListVersionsOffline(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.catalog.err = errors.New("繋がりません")

	got, err := h.uc.ListVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Error("取れていないのに Available")
	}
	if !slices.Equal(got.Versions, []string{"26.2"}) {
		t.Errorf("版が %v。現在の版だけのはず", got.Versions)
	}
	if got.UnavailableReason == "" {
		t.Error("理由が空")
	}
}

// --- 作成 ---

func TestCreateWritesVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Create(context.Background(), worldctl.CreateOptions{
		Name: "legacy", Version: "1.21.4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := h.wait(t, handle.ID()); snap.State != operation.StateSucceeded {
		t.Fatalf("失敗した: %s", snap.ErrorMessage)
	}
	if got := h.config.get("MC_VERSION"); got != "1.21.4" {
		t.Errorf("MC_VERSION が %q", got)
	}
}

// Paper に無い版はサーバーを止める前に断る。書くと起動しない。
func TestCreateRejectsUnknownVersionBeforeStopping(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	_, err := h.uc.Create(context.Background(), worldctl.CreateOptions{
		Name: "fresh", Version: "26.1",
	})
	if !errors.Is(err, worldctl.ErrVersionUnavailable) {
		t.Fatalf("ErrVersionUnavailable を期待したが %v", err)
	}
	if got := h.calls(); got != "" {
		t.Errorf("サーバーに触っている: %q", got)
	}
}

// 一覧が取れないときは、現在の版でだけ作れる。
func TestCreateOfflineAllowsOnlyCurrentVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")
	h.catalog.err = errors.New("繋がりません")

	if _, err := h.uc.Create(context.Background(), worldctl.CreateOptions{
		Name: "other", Version: "26.3",
	}); !errors.Is(err, worldctl.ErrCatalogUnavailable) {
		t.Fatalf("ErrCatalogUnavailable を期待したが %v", err)
	}

	handle, err := h.uc.Create(context.Background(), worldctl.CreateOptions{
		Name: "same", Version: "26.2",
	})
	if err != nil {
		t.Fatalf("現在の版なのに断られた: %v", err)
	}
	h.wait(t, handle.ID())
}

// 版を指定しなければ触らない。
func TestCreateKeepsVersionWhenUnspecified(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world")

	handle, err := h.uc.Create(context.Background(), worldctl.CreateOptions{Name: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, handle.ID())
	if got := h.config.get("MC_VERSION"); got != "26.2" {
		t.Errorf("MC_VERSION が %q", got)
	}
}

// --- 切り替え ---

// 古いワールドへ切り替えたら、サーバーの版もそのワールドに合わせる。
// 合わせないと、今の版で開かれて勝手に上がる（元に戻せない）。
func TestSwitchFollowsWorldVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "legacy")
	h.levels.settings["legacy"] = port.LevelSettings{HasVersion: true, Version: "1.21.4"}
	h.levels.settings["world"] = port.LevelSettings{HasVersion: true, Version: "26.2"}

	h.switchTo(t, "legacy")
	if got := h.config.get("MC_VERSION"); got != "1.21.4" {
		t.Errorf("MC_VERSION が %q。1.21.4 のはず", got)
	}

	// 戻れば版も戻る。
	h.switchTo(t, "world")
	if got := h.config.get("MC_VERSION"); got != "26.2" {
		t.Errorf("MC_VERSION が %q。26.2 に戻るはず", got)
	}
}

// 読めないとき（level.dat が無い、スナップショットで開いた）は触らない。
func TestSwitchKeepsVersionWhenUnreadable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "snapshot")

	h.switchTo(t, "snapshot")
	if got := h.config.get("MC_VERSION"); got != "26.2" {
		t.Errorf("MC_VERSION が %q。触らないはず", got)
	}
}

// Paper に無い版のワールドへは、止める前に断る。起動しないか、
// 今の版で開いて勝手に上がるかのどちらかになるため。
func TestSwitchRefusesUnavailableVersionBeforeStopping(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "old")
	h.levels.settings["old"] = port.LevelSettings{HasVersion: true, Version: "26.1"}

	_, err := h.uc.Switch(context.Background(), "old")
	if !errors.Is(err, worldctl.ErrVersionUnavailable) {
		t.Fatalf("ErrVersionUnavailable を期待したが %v", err)
	}
	if got := h.calls(); got != "" {
		t.Errorf("サーバーに触っている: %q", got)
	}
}

// 一覧が取れないとき（オフライン）は通す。起動できなければ元へ切り替え
// 直せば版も戻る。止めてしまうと、オフラインの間は一切切り替えられない。
func TestSwitchOfflineStillFollowsVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, "world", "legacy")
	h.catalog.err = errors.New("繋がりません")
	h.levels.settings["legacy"] = port.LevelSettings{HasVersion: true, Version: "1.21.4"}

	h.switchTo(t, "legacy")
	if got := h.config.get("MC_VERSION"); got != "1.21.4" {
		t.Errorf("MC_VERSION が %q", got)
	}
}
