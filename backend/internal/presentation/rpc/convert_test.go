package rpc

import (
	"testing"
	"time"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// 読めなかったバージョンを転送形式にしたとき、
// readable=false であって「バージョン 0」ではないこと。
func TestWorldVersionToProto(t *testing.T) {
	t.Parallel()

	t.Run("読めた場合", func(t *testing.T) {
		t.Parallel()
		dv, err := shared.NewDataVersion(4903)
		if err != nil {
			t.Fatal(err)
		}
		got := worldVersionToProto(shared.NewWorldVersion("26.2", dv, false, "world"))

		if !got.GetReadable() {
			t.Error("Readable が偽")
		}
		if got.GetDataVersion() != 4903 {
			t.Errorf("DataVersion が %d", got.GetDataVersion())
		}
		if got.GetName() != "26.2" || got.GetLevelName() != "world" {
			t.Errorf("Name=%q LevelName=%q", got.GetName(), got.GetLevelName())
		}
	})

	t.Run("読めなかった場合", func(t *testing.T) {
		t.Parallel()
		got := worldVersionToProto(shared.UnreadableWorldVersion())

		if got.GetReadable() {
			t.Error("Readable が真")
		}
		if got.GetDataVersion() != 0 || got.GetName() != "" {
			t.Error("読めていないのに値が入っている")
		}
	})
}

// 変換の網羅。取りこぼすと画面に「不明」が出続ける。
func TestEnumConversionsAreExhaustive(t *testing.T) {
	t.Parallel()

	t.Run("ContainerState", func(t *testing.T) {
		t.Parallel()
		cases := map[server.ContainerState]mcadminv1.ContainerState{
			server.ContainerRunning:    mcadminv1.ContainerState_CONTAINER_STATE_RUNNING,
			server.ContainerExited:     mcadminv1.ContainerState_CONTAINER_STATE_EXITED,
			server.ContainerRestarting: mcadminv1.ContainerState_CONTAINER_STATE_RESTARTING,
			server.ContainerMissing:    mcadminv1.ContainerState_CONTAINER_STATE_MISSING,
		}
		for in, want := range cases {
			if got := containerStateToProto(in); got != want {
				t.Errorf("%v → %v。%v のはず", in, got, want)
			}
		}
	})

	t.Run("OperationKind", func(t *testing.T) {
		t.Parallel()
		kinds := []operation.Kind{
			operation.KindServerStart, operation.KindServerStop, operation.KindServerRestart,
			operation.KindBackupCreate, operation.KindBackupRestore,
			operation.KindWorldSwitch, operation.KindWorldCreate, operation.KindWorldClone,
			operation.KindWorldRename, operation.KindWorldDelete,
		}
		seen := map[mcadminv1.OperationKind]bool{}
		for _, k := range kinds {
			got := operationKindToProto(k)
			if got == mcadminv1.OperationKind_OPERATION_KIND_UNSPECIFIED {
				t.Errorf("%v が未指定に落ちている", k)
			}
			if seen[got] {
				t.Errorf("%v の変換先が重複している: %v", k, got)
			}
			seen[got] = true
		}
	})

	t.Run("OperationState", func(t *testing.T) {
		t.Parallel()
		cases := map[operation.State]mcadminv1.OperationState{
			operation.StatePending:   mcadminv1.OperationState_OPERATION_STATE_PENDING,
			operation.StateRunning:   mcadminv1.OperationState_OPERATION_STATE_RUNNING,
			operation.StateSucceeded: mcadminv1.OperationState_OPERATION_STATE_SUCCEEDED,
			operation.StateFailed:    mcadminv1.OperationState_OPERATION_STATE_FAILED,
		}
		for in, want := range cases {
			if got := operationStateToProto(in); got != want {
				t.Errorf("%v → %v", in, got)
			}
		}
	})

	t.Run("SavingState と LogLevel", func(t *testing.T) {
		t.Parallel()
		if got := savingStateToProto(server.SavingSuspectOff); got != mcadminv1.SavingState_SAVING_STATE_SUSPECT_OFF {
			t.Errorf("SavingSuspectOff → %v", got)
		}
		if got := savingStateToProto(server.SavingAssumedOn); got != mcadminv1.SavingState_SAVING_STATE_ASSUMED_ON {
			t.Errorf("SavingAssumedOn → %v", got)
		}
		if got := logLevelToProto(operation.LevelError); got != mcadminv1.LogLevel_LOG_LEVEL_ERROR {
			t.Errorf("LevelError → %v", got)
		}
		if got := logLevelToProto(operation.LevelWarn); got != mcadminv1.LogLevel_LOG_LEVEL_WARN {
			t.Errorf("LevelWarn → %v", got)
		}
	})
}

// 未完了の操作で FinishedAt を送らない。
// エポック 0 を送ると画面に 1970 年と出る。
func TestOperationToProtoOmitsZeroTimes(t *testing.T) {
	t.Parallel()

	snap := operation.Snapshot{
		ID:        operation.MustID("op-1"),
		Kind:      operation.KindBackupCreate,
		State:     operation.StateRunning,
		StartedAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		StepIndex: 1,
		StepTotal: 3,
	}

	got := operationToProto(snap)
	if got.GetFinishedAt() != nil {
		t.Error("未完了なのに FinishedAt が入っている")
	}
	if got.GetStartedAt() == nil {
		t.Error("StartedAt が入っていない")
	}
	if got.GetId() != "op-1" || got.GetStepTotal() != 3 {
		t.Errorf("Id=%q StepTotal=%d", got.GetId(), got.GetStepTotal())
	}
}

// イベントは完全な状態を同梱する。1 つ取りこぼしても表示がずれない。
func TestOperationEventToProtoCarriesSnapshot(t *testing.T) {
	t.Parallel()

	op, err := operation.New(operation.MustID("op-1"), operation.KindWorldSwitch,
		[]string{"a", "b"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Advance(time.Now()); err != nil {
		t.Fatal(err)
	}

	events := op.Events()
	got := operationEventToProto(events[len(events)-1])

	if got.GetSeq() != 1 {
		t.Errorf("Seq が %d", got.GetSeq())
	}
	if got.GetSnapshot() == nil {
		t.Fatal("Snapshot が入っていない")
	}
	if got.GetSnapshot().GetStepIndex() != 1 {
		t.Errorf("Snapshot の StepIndex が %d", got.GetSnapshot().GetStepIndex())
	}
	if got.GetAt() == nil {
		t.Error("At が入っていない")
	}
}
