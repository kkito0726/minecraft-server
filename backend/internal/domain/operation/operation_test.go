package operation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

func newOp(t *testing.T, steps ...string) *operation.Operation {
	t.Helper()

	if len(steps) == 0 {
		steps = []string{"準備", "実行", "確認"}
	}
	op, err := operation.New(
		operation.MustID(t.Name()),
		operation.KindBackupCreate,
		steps,
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func TestNewRequiresSteps(t *testing.T) {
	t.Parallel()

	_, err := operation.New(operation.MustID("x"), operation.KindBackupCreate, nil, time.Now())
	if err == nil {
		t.Error("ステップが無い操作は作れないはず")
	}
}

func TestNewStartsPending(t *testing.T) {
	t.Parallel()

	op := newOp(t)

	if op.State() != operation.StatePending {
		t.Errorf("初期状態が %v。PENDING のはず", op.State())
	}
	if op.StepTotal() != 3 {
		t.Errorf("StepTotal が %d", op.StepTotal())
	}
	if op.StepIndex() != 0 {
		t.Errorf("StepIndex が %d。開始前は 0 のはず", op.StepIndex())
	}
	if !op.FinishedAt().IsZero() {
		t.Error("未完了なのに FinishedAt が設定されている")
	}
}

// ステップを進めると RUNNING になり、番号と名前が追随する。
func TestAdvance(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Date(2026, 9, 10, 0, 0, 1, 0, time.UTC)

	if err := op.Advance(at); err != nil {
		t.Fatalf("Advance に失敗: %v", err)
	}
	if op.State() != operation.StateRunning {
		t.Errorf("State が %v。RUNNING のはず", op.State())
	}
	if op.StepIndex() != 1 {
		t.Errorf("StepIndex が %d。1 のはず", op.StepIndex())
	}
	if op.CurrentStep() != "準備" {
		t.Errorf("CurrentStep が %q", op.CurrentStep())
	}

	_ = op.Advance(at)
	if op.CurrentStep() != "実行" {
		t.Errorf("2 回目の CurrentStep が %q", op.CurrentStep())
	}
}

// 総ステップ数を超えて進められない。表示が n/N を超えると意味が壊れる。
func TestAdvanceBeyondTotal(t *testing.T) {
	t.Parallel()

	op := newOp(t, "a", "b")
	at := time.Now()

	_ = op.Advance(at)
	_ = op.Advance(at)
	if err := op.Advance(at); err == nil {
		t.Error("総数を超えて進められるべきでない")
	}
	if op.StepIndex() != 2 {
		t.Errorf("StepIndex が %d。2 のままのはず", op.StepIndex())
	}
}

// 状態遷移は一方向。終端に達したら二度と動かない。
// 完了した操作が「実行中」に戻ると、UI の排他表示が永久に解けなくなる。
func TestTerminalStatesAreFinal(t *testing.T) {
	t.Parallel()

	at := time.Now()

	t.Run("成功後は動かない", func(t *testing.T) {
		t.Parallel()
		op := newOp(t)
		_ = op.Advance(at)
		if err := op.Succeed(at); err != nil {
			t.Fatal(err)
		}

		if err := op.Advance(at); !errors.Is(err, operation.ErrAlreadyFinished) {
			t.Errorf("Advance が %v", err)
		}
		if err := op.Succeed(at); !errors.Is(err, operation.ErrAlreadyFinished) {
			t.Errorf("Succeed が %v", err)
		}
		if err := op.Fail(at, "E", "失敗"); !errors.Is(err, operation.ErrAlreadyFinished) {
			t.Errorf("Fail が %v", err)
		}
		if op.State() != operation.StateSucceeded {
			t.Errorf("State が %v", op.State())
		}
	})

	t.Run("失敗後は動かない", func(t *testing.T) {
		t.Parallel()
		op := newOp(t)
		_ = op.Advance(at)
		if err := op.Fail(at, "E_DISK", "空き容量が不足しています"); err != nil {
			t.Fatal(err)
		}

		if err := op.Succeed(at); !errors.Is(err, operation.ErrAlreadyFinished) {
			t.Errorf("Succeed が %v", err)
		}
		if op.State() != operation.StateFailed {
			t.Errorf("State が %v", op.State())
		}
		if op.ErrorCode() != "E_DISK" {
			t.Errorf("ErrorCode が %q", op.ErrorCode())
		}
		if op.ErrorMessage() != "空き容量が不足しています" {
			t.Errorf("ErrorMessage が %q", op.ErrorMessage())
		}
	})
}

func TestFinishedAtIsSet(t *testing.T) {
	t.Parallel()

	finished := time.Date(2026, 9, 10, 0, 5, 0, 0, time.UTC)
	op := newOp(t)
	_ = op.Advance(time.Now())
	_ = op.Succeed(finished)

	if !op.FinishedAt().Equal(finished) {
		t.Errorf("FinishedAt が %v", op.FinishedAt())
	}
	if !op.IsFinished() {
		t.Error("IsFinished が偽")
	}
}

// イベントの seq は 1 始まりで単調増加する。
// 飛んだり戻ったりすると、クライアントの再接続位置がずれて
// ログの取りこぼしや二重表示が起きる。
func TestEventSequenceIsMonotonic(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()

	_ = op.Advance(at)
	op.Log(at, operation.LevelInfo, "1 行目")
	op.Log(at, operation.LevelWarn, "2 行目")
	_ = op.Advance(at)
	_ = op.Succeed(at)

	events := op.Events()
	if len(events) < 4 {
		t.Fatalf("イベントが %d 件。少なすぎる", len(events))
	}
	for i, e := range events {
		if want := int64(i + 1); e.Seq() != want {
			t.Errorf("%d 番目の Seq が %d。%d のはず", i, e.Seq(), want)
		}
	}
}

// 各イベントは操作の完全な状態を同梱する。
// 1 つ取りこぼしても表示がずれないための不変条件。
func TestEventCarriesSnapshot(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()

	_ = op.Advance(at)
	op.Log(at, operation.LevelInfo, "処理中")

	events := op.Events()
	last := events[len(events)-1]

	snap := last.Snapshot()
	if snap.State != operation.StateRunning {
		t.Errorf("snapshot の State が %v", snap.State)
	}
	if snap.StepIndex != 1 {
		t.Errorf("snapshot の StepIndex が %d", snap.StepIndex)
	}
	if snap.CurrentStep != "準備" {
		t.Errorf("snapshot の CurrentStep が %q", snap.CurrentStep)
	}
}

// snapshot は取得時点の値を固定する。後から操作が進んでも変わらない。
func TestSnapshotIsIndependent(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()

	_ = op.Advance(at)
	op.Log(at, operation.LevelInfo, "1 段目")
	first := op.Events()[len(op.Events())-1].Snapshot()

	_ = op.Advance(at)
	op.Log(at, operation.LevelInfo, "2 段目")

	if first.StepIndex != 1 {
		t.Errorf("後の操作で過去の snapshot が変わった: StepIndex=%d", first.StepIndex)
	}
	if first.CurrentStep != "準備" {
		t.Errorf("後の操作で過去の snapshot が変わった: CurrentStep=%q", first.CurrentStep)
	}
}

// Events() が返すスライスを変更しても、集約の中身は変わらない。
func TestEventsIsDefensiveCopy(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()
	_ = op.Advance(at)

	events := op.Events()
	before := len(events)
	events = append(events, operation.Event{})
	_ = events

	if len(op.Events()) != before {
		t.Error("外から Events を変更できてしまう")
	}
}

// 進捗のバイト数は表示に使う。総量が不明な場合は 0 のまま。
func TestBytes(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()
	_ = op.Advance(at)

	op.SetBytes(at, 512, 2048)
	done, total := op.Bytes()
	if done != 512 || total != 2048 {
		t.Errorf("Bytes が %d/%d", done, total)
	}
}

// 属性は操作固有の情報を持ち回るために使う。
// 復元なら退避先のパス、バックアップなら生成したファイル名。
func TestAttributes(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	op.SetAttribute("quarantine_path", "data/world.broken-20260910-000000")

	attrs := op.Attributes()
	if attrs["quarantine_path"] != "data/world.broken-20260910-000000" {
		t.Errorf("属性が %v", attrs)
	}

	// 外から変更できないこと
	attrs["quarantine_path"] = "書き換え"
	if op.Attributes()["quarantine_path"] == "書き換え" {
		t.Error("外から属性を変更できてしまう")
	}
}

// 終了したイベントは、それが最後であることが分かること。
func TestFinalEventReflectsTerminalState(t *testing.T) {
	t.Parallel()

	op := newOp(t)
	at := time.Now()
	_ = op.Advance(at)
	_ = op.Fail(at, "E_LOCK", "他の操作が実行中です")

	last := op.Events()[len(op.Events())-1]
	snap := last.Snapshot()

	if snap.State != operation.StateFailed {
		t.Errorf("最後のイベントの State が %v", snap.State)
	}
	if snap.ErrorMessage != "他の操作が実行中です" {
		t.Errorf("最後のイベントに失敗理由が入っていない: %q", snap.ErrorMessage)
	}
}
