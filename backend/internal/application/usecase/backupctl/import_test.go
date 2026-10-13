package backupctl_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

/*
取り込みはワールドに触らない。保管先が 1 件増えるだけ。

差し替えるかどうかは、このあと復元の画面でこれまでどおりの関門
（バージョンの確認と名前の入力）を通して決める。「置くこと」と
「使うこと」を同時に決めさせないための切り分け。
*/
func TestImportAdoptsArchiveUnderData(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.staged = &fakeStaged{entries: []string{"data/world/level.dat"}, total: 2048}

	got, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	if !h.store.staged.adopted {
		t.Error("保管先へ確定していない")
	}
	if got.Level != "world" {
		t.Errorf("ワールド名が %q", got.Level)
	}
	// ワールドには触らない。
	if calls := h.calls(); strings.Contains(calls, "Extract") || strings.Contains(calls, "Down") {
		t.Errorf("ワールドやサーバーに触っている: %v", calls)
	}
}

/*
ワールド名から始まる形も、中身を触らず受け取る。

2026-10 以降の保管形式そのものであり、配布ワールドもこの形。復元が
両方の形を受けるようになったので、入口で包み直さない（最大 200MB の
全コピーを避ける）。
*/
func TestImportAcceptsWorldRootArchiveWithoutRewrap(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.staged = &fakeStaged{entries: []string{"MyWorld/level.dat"}, total: 2048}

	got, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	if !h.store.staged.adopted {
		t.Error("保管先へ確定していない")
	}
	if got.Level != "MyWorld" {
		t.Errorf("ワールド名が %q", got.Level)
	}
}

// ファイル名には版とワールド名を入れる。どのワールドのどの版かが
// ファイル名から分かるようにするため（規約は取得したものと同じ）。
func TestImportNamesArchiveWithVersionAndLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.staged = &fakeStaged{entries: []string{"data/world/level.dat"}, total: 2048}

	got, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	name := got.ID.String()
	for _, want := range []string{"26.2", "world", "imported", ".zip"} {
		if !strings.Contains(name, want) {
			t.Errorf("ファイル名に %q が入っていない: %s", want, name)
		}
	}
	parsed := backup.ParseName(name)
	if parsed.Level != "world" {
		t.Errorf("付けた名前を読み戻せない: %+v", parsed)
	}
}

// 版を読めなくても取り込みは止めない。
// 復元の画面が「不明」として承諾を求めるので、安全側には倒れている。
func TestImportAcceptsUnreadableVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.levels.version = shared.UnreadableWorldVersion()
	h.store.staged = &fakeStaged{entries: []string{"data/world/level.dat"}, total: 2048}

	got, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version.Readable() {
		t.Error("読めないはずの版が読めている")
	}
	if !strings.Contains(got.ID.String(), "unknown") {
		t.Errorf("版が不明であることがファイル名に出ていない: %s", got.ID)
	}
}

// 取り込めないものを受け取ると、保管先に使えないファイルが増える。
func TestImportRejectsArchiveWithoutWorld(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.staged = &fakeStaged{entries: nil, total: 10}

	if _, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024); !errors.Is(
		err, backup.ErrNotImportable) {
		t.Fatalf("受理された、または別のエラー: %v", err)
	}
	if !h.store.staged.discarded {
		t.Error("拒否したのに一時ファイルを捨てていない")
	}
}

/*
Pi ではディスクを埋めきるとサーバーごと止まる。

送られてくる大きさが分かっているなら、1 バイトも受け取らずに落とす。
見込むのは 1 つ分。一時ファイルへ書いて rename で確定するだけなので、
2 つ分の場所は要らない（包み直しをやめた）。
*/
func TestImportRefusesWhenDiskIsTight(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.worlds.available = 1000

	if _, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1001); !errors.Is(
		err, port.ErrInsufficientSpace) {
		t.Fatalf("空きが足りないのに受理した: %v", err)
	}
	if h.store.staged != nil {
		t.Error("受け取る前に落としていない")
	}
}

/*
空きと同じ大きさまでは受け取る。

包み直していた頃は半分までしか受け取れなかった。入口の全コピーを
やめたぶん、同じ空き容量でより大きなアーカイブを取り込める。
*/
func TestImportAcceptsArchiveUpToAvailableSpace(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.worlds.available = 1000
	h.store.staged = &fakeStaged{entries: []string{"data/world/level.dat"}, total: 900}

	if _, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 900); err != nil {
		t.Fatalf("空きに収まるのに断られた: %v", err)
	}
}

// 大きさが分からなくても、空き容量を上限にして打ち切れるようにする。
func TestImportPassesLimitToStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.worlds.available = 4000
	h.store.staged = &fakeStaged{entries: []string{"data/world/level.dat"}, total: 10}

	if _, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 0); err != nil {
		t.Fatal(err)
	}
	// 包み直しをやめたので、上限は空き容量そのもの。
	if h.store.stagedLimit != 4000 {
		t.Errorf("上限が %d（期待 4000）", h.store.stagedLimit)
	}
}

// 確定に失敗したときも一時ファイルを残さない。
func TestImportDiscardsOnAdoptFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t, true, nil)
	h.store.staged = &fakeStaged{
		entries:  []string{"data/world/level.dat"},
		total:    2048,
		adoptErr: errors.New("置けません"),
	}

	if _, err := h.uc.Import(t.Context(), strings.NewReader("zip"), 1024); err == nil {
		t.Fatal("エラーが返っていない")
	}
	if !h.store.staged.discarded {
		t.Error("失敗したのに一時ファイルを捨てていない")
	}
}
