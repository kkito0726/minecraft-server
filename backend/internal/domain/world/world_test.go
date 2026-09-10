package world_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

func mustName(t *testing.T, s string) world.Name {
	t.Helper()

	n, err := world.NewName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWorldAccessors(t *testing.T) {
	t.Parallel()

	dv, err := shared.NewDataVersion(4903)
	if err != nil {
		t.Fatal(err)
	}
	played := time.Date(2026, 9, 10, 0, 50, 28, 0, time.UTC)
	version := shared.NewWorldVersion("26.2", dv, false, "world")

	w := world.NewWorld(mustName(t, "world"), true, 30<<20, played, version, true)

	if w.Name().String() != "world" {
		t.Errorf("Name() が %q", w.Name())
	}
	if !w.IsActive() {
		t.Error("IsActive() が偽")
	}
	if w.SizeBytes() != 30<<20 {
		t.Errorf("SizeBytes() が %d", w.SizeBytes())
	}
	if !w.LastPlayed().Equal(played) {
		t.Errorf("LastPlayed() が %v", w.LastPlayed())
	}
	if !w.Version().Readable() {
		t.Error("Version() が読めない扱いになっている")
	}
	if !w.HasSessionLock() {
		t.Error("HasSessionLock() が偽")
	}
}

// 稼働中のワールドを消すと復旧できない。先に切り替えてもらう。
func TestWorldCanDelete(t *testing.T) {
	t.Parallel()

	version := shared.UnreadableWorldVersion()

	active := world.NewWorld(mustName(t, "world"), true, 0, time.Time{}, version, false)
	if err := active.CanDelete(); !errors.Is(err, world.ErrActiveWorld) {
		t.Errorf("稼働中は拒否されるはず。err=%v", err)
	}

	inactive := world.NewWorld(mustName(t, "creative"), false, 0, time.Time{}, version, false)
	if err := inactive.CanDelete(); err != nil {
		t.Errorf("非稼働は削除できるはず。err=%v", err)
	}
}
