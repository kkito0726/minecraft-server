package versionsync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/versionsync"
)

// fakeSnapshot は .env の読み込み結果の偽物。不変に振る舞う。
type fakeSnapshot struct{ values map[string]string }

func (f *fakeSnapshot) Get(key string) (string, bool) {
	v, ok := f.values[key]
	return v, ok
}

func (f *fakeSnapshot) With(key, value string) port.ConfigSnapshot {
	copied := map[string]string{}
	for k, v := range f.values {
		copied[k] = v
	}
	copied[key] = value
	return &fakeSnapshot{values: copied}
}

func snapshotOf(version string) port.ConfigSnapshot {
	return &fakeSnapshot{values: map[string]string{versionsync.KeyVersion: version}}
}

// fakeConfig は .env の読み書きの偽物。
type fakeConfig struct {
	snapshot port.ConfigSnapshot
	err      error
}

func (f *fakeConfig) Load(context.Context) (port.ConfigSnapshot, error) {
	return f.snapshot, f.err
}
func (f *fakeConfig) Save(context.Context, port.ConfigSnapshot) error { return nil }

// fakeCatalog は Paper が配っている版の一覧の偽物。
type fakeCatalog struct {
	versions []string
	err      error
}

func (f *fakeCatalog) StableVersions(context.Context) ([]string, error) {
	return f.versions, f.err
}

// --- Follow ---

/*
版が違えば合わせる。

合わせないと、古いワールドは今の版で開かれて勝手に上がり（元に戻せない）、
新しいワールドは古い版では開けずにサーバーが起動しない。
*/
func TestFollowWritesVersionWhenItDiffers(t *testing.T) {
	t.Parallel()

	current := snapshotOf("26.2")
	next, note := versionsync.Follow(current, current,
		port.LevelSettings{Version: "26.3", HasVersion: true})

	if got, _ := next.Get(versionsync.KeyVersion); got != "26.3" {
		t.Errorf("%s が %q。26.3 のはず", versionsync.KeyVersion, got)
	}
	if note == "" {
		t.Error("注意書きが空。版が変わることを黙って済ませてはならない")
	}
	// クライアント側も人が合わせる必要がある。そこが伝わること。
	if !strings.Contains(note, "クライアント") {
		t.Errorf("注意書きが %q。クライアントを合わせる話が無い", note)
	}
}

// 同じ版なら触らない。書くたびに .env の更新時刻が動くと、
// 人が編集したのかどうかの手がかりが消える。
func TestFollowLeavesSameVersionAlone(t *testing.T) {
	t.Parallel()

	current := snapshotOf("26.2")
	next, note := versionsync.Follow(current, current,
		port.LevelSettings{Version: "26.2", HasVersion: true})

	if next != current {
		t.Error("同じ版なのに差し替えている")
	}
	if note != "" {
		t.Errorf("注意書きが %q。同じ版なら何も言わない", note)
	}
}

// 前後の空白は無視して比べる。人が vi で編集した .env に空白が入りうる。
func TestFollowIgnoresSurroundingSpaces(t *testing.T) {
	t.Parallel()

	current := snapshotOf("  26.2  ")
	_, note := versionsync.Follow(current, current,
		port.LevelSettings{Version: "26.2", HasVersion: true})

	if note != "" {
		t.Errorf("注意書きが %q。空白の違いだけで書き換えてはならない", note)
	}
}

/*
読めないときは触らない。

level.dat が無い、あるいはスナップショットで開かれたワールドがこれ。
Paper はスナップショットを配らないので、書いても起動しなくなるだけ。
*/
func TestFollowDoesNothingWhenVersionIsUnreadable(t *testing.T) {
	t.Parallel()

	current := snapshotOf("26.2")
	next, note := versionsync.Follow(current, current, port.LevelSettings{})

	if next != current {
		t.Error("読めないのに差し替えている")
	}
	if note != "" {
		t.Errorf("注意書きが %q。読めないなら何も言わない", note)
	}
}

// --- CheckAvailable ---

// Paper が配っていない版は断る。書くとサーバーが起動しない。
func TestCheckAvailableRefusesUnlistedVersion(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{versions: []string{"26.2", "26.1"}}

	err := versionsync.CheckAvailable(context.Background(), catalog, "26.2", "26.9")
	if !errors.Is(err, versionsync.ErrVersionUnavailable) {
		t.Fatalf("err = %v。ErrVersionUnavailable のはず", err)
	}
	// 何をすればよいかが書かれていること。
	if !strings.Contains(err.Error(), "runbook") {
		t.Errorf("err = %v。手順書への案内が無い", err)
	}
}

func TestCheckAvailableAcceptsListedVersion(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{versions: []string{"26.3", "26.2"}}

	if err := versionsync.CheckAvailable(context.Background(), catalog, "26.2", "26.3"); err != nil {
		t.Errorf("一覧にある版が断られた: %v", err)
	}
}

// 今動いている版は、一覧に無くても通す。今まさに起動している版なので。
func TestCheckAvailableAlwaysAcceptsCurrentVersion(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{versions: []string{"26.1"}}

	if err := versionsync.CheckAvailable(context.Background(), catalog, "26.2", "26.2"); err != nil {
		t.Errorf("現在の版が断られた: %v", err)
	}
}

/*
一覧が取れないときは通す。

itzg のイメージがその版の jar を既に持っていれば起動できる。ここで
止めると、オフラインの間は一切切り替え・復元ができなくなる。
*/
func TestCheckAvailablePassesWhenCatalogFails(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{err: errors.New("繋がりません")}

	if err := versionsync.CheckAvailable(context.Background(), catalog, "26.2", "26.9"); err != nil {
		t.Errorf("オフラインで断られた: %v", err)
	}
}

// 一覧を持たない構成でも通す。塞ぐ理由がない。
func TestCheckAvailablePassesWithoutCatalog(t *testing.T) {
	t.Parallel()

	if err := versionsync.CheckAvailable(context.Background(), nil, "26.2", "26.9"); err != nil {
		t.Errorf("一覧なしで断られた: %v", err)
	}
}

// 版が分からないときは確かめようがないので通す。
func TestCheckAvailablePassesWhenWantIsEmpty(t *testing.T) {
	t.Parallel()

	catalog := &fakeCatalog{versions: []string{"26.2"}}

	if err := versionsync.CheckAvailable(context.Background(), catalog, "26.2", ""); err != nil {
		t.Errorf("版が空で断られた: %v", err)
	}
}

// --- Current ---

func TestCurrentReadsVersion(t *testing.T) {
	t.Parallel()

	cfg := &fakeConfig{snapshot: snapshotOf("  26.2 ")}

	got, err := versionsync.Current(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "26.2" {
		t.Errorf("Current() = %q。前後の空白を落とすはず", got)
	}
}

func TestCurrentPropagatesLoadError(t *testing.T) {
	t.Parallel()

	want := errors.New("読めません")
	cfg := &fakeConfig{err: want}

	if _, err := versionsync.Current(context.Background(), cfg); !errors.Is(err, want) {
		t.Errorf("err = %v。読み込みの失敗を伝えるはず", err)
	}
}
