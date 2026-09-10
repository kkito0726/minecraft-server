// Package rpc は Connect のハンドラ。proto とドメインの変換を受け持つ。
//
// proto はあくまで転送の形式であってドメインの表現ではない。
// この層が両者を変換することで、proto を変えてもドメインが壊れず、
// その逆も成り立つ。
package rpc

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// worldVersionToProto は level.dat のバージョンを転送形式にする。
func worldVersionToProto(v shared.WorldVersion) *mcadminv1.WorldVersion {
	out := &mcadminv1.WorldVersion{Readable: v.Readable()}
	if !v.Readable() {
		return out
	}
	dv, _ := v.DataVersion()
	out.Name = v.Name()
	out.DataVersion = dv.Int32()
	out.Snapshot = v.Snapshot()
	out.LevelName = v.LevelName()
	return out
}

// containerStateToProto はコンテナの状態を転送形式にする。
func containerStateToProto(s server.ContainerState) mcadminv1.ContainerState {
	switch s {
	case server.ContainerRunning:
		return mcadminv1.ContainerState_CONTAINER_STATE_RUNNING
	case server.ContainerExited:
		return mcadminv1.ContainerState_CONTAINER_STATE_EXITED
	case server.ContainerRestarting:
		return mcadminv1.ContainerState_CONTAINER_STATE_RESTARTING
	case server.ContainerMissing:
		return mcadminv1.ContainerState_CONTAINER_STATE_MISSING
	default:
		return mcadminv1.ContainerState_CONTAINER_STATE_UNSPECIFIED
	}
}

// savingStateToProto は保存状態の推定値を転送形式にする。
func savingStateToProto(s server.SavingState) mcadminv1.SavingState {
	if s == server.SavingSuspectOff {
		return mcadminv1.SavingState_SAVING_STATE_SUSPECT_OFF
	}
	return mcadminv1.SavingState_SAVING_STATE_ASSUMED_ON
}

// operationKindToProto は操作の種類を転送形式にする。
func operationKindToProto(k operation.Kind) mcadminv1.OperationKind {
	switch k {
	case operation.KindServerStart:
		return mcadminv1.OperationKind_OPERATION_KIND_SERVER_START
	case operation.KindServerStop:
		return mcadminv1.OperationKind_OPERATION_KIND_SERVER_STOP
	case operation.KindServerRestart:
		return mcadminv1.OperationKind_OPERATION_KIND_SERVER_RESTART
	case operation.KindBackupCreate:
		return mcadminv1.OperationKind_OPERATION_KIND_BACKUP_CREATE
	case operation.KindBackupRestore:
		return mcadminv1.OperationKind_OPERATION_KIND_BACKUP_RESTORE
	case operation.KindWorldSwitch:
		return mcadminv1.OperationKind_OPERATION_KIND_WORLD_SWITCH
	case operation.KindWorldCreate:
		return mcadminv1.OperationKind_OPERATION_KIND_WORLD_CREATE
	case operation.KindWorldClone:
		return mcadminv1.OperationKind_OPERATION_KIND_WORLD_CLONE
	case operation.KindWorldRename:
		return mcadminv1.OperationKind_OPERATION_KIND_WORLD_RENAME
	case operation.KindWorldDelete:
		return mcadminv1.OperationKind_OPERATION_KIND_WORLD_DELETE
	default:
		return mcadminv1.OperationKind_OPERATION_KIND_UNSPECIFIED
	}
}

// operationStateToProto は操作の状態を転送形式にする。
func operationStateToProto(s operation.State) mcadminv1.OperationState {
	switch s {
	case operation.StatePending:
		return mcadminv1.OperationState_OPERATION_STATE_PENDING
	case operation.StateRunning:
		return mcadminv1.OperationState_OPERATION_STATE_RUNNING
	case operation.StateSucceeded:
		return mcadminv1.OperationState_OPERATION_STATE_SUCCEEDED
	case operation.StateFailed:
		return mcadminv1.OperationState_OPERATION_STATE_FAILED
	default:
		return mcadminv1.OperationState_OPERATION_STATE_UNSPECIFIED
	}
}

// logLevelToProto はログの深刻度を転送形式にする。
func logLevelToProto(l operation.Level) mcadminv1.LogLevel {
	switch l {
	case operation.LevelWarn:
		return mcadminv1.LogLevel_LOG_LEVEL_WARN
	case operation.LevelError:
		return mcadminv1.LogLevel_LOG_LEVEL_ERROR
	default:
		return mcadminv1.LogLevel_LOG_LEVEL_INFO
	}
}

// operationToProto は操作の状態を転送形式にする。
func operationToProto(s operation.Snapshot) *mcadminv1.Operation {
	out := &mcadminv1.Operation{
		Id:           s.ID.String(),
		Kind:         operationKindToProto(s.Kind),
		State:        operationStateToProto(s.State),
		StepIndex:    int32(s.StepIndex),
		StepTotal:    int32(s.StepTotal),
		CurrentStep:  s.CurrentStep,
		StepNames:    s.StepNames,
		BytesDone:    s.BytesDone,
		BytesTotal:   s.BytesTotal,
		ErrorCode:    s.ErrorCode,
		ErrorMessage: s.ErrorMessage,
		Attributes:   s.Attributes,
	}
	if !s.StartedAt.IsZero() {
		out.StartedAt = timestamppb.New(s.StartedAt)
	}
	if !s.FinishedAt.IsZero() {
		out.FinishedAt = timestamppb.New(s.FinishedAt)
	}
	return out
}

// operationEventToProto は進捗イベントを転送形式にする。
func operationEventToProto(e operation.Event) *mcadminv1.WatchOperationResponse {
	out := &mcadminv1.WatchOperationResponse{
		Seq:      e.Seq(),
		Level:    logLevelToProto(e.Level()),
		Message:  e.Message(),
		Snapshot: operationToProto(e.Snapshot()),
	}
	if !e.At().IsZero() {
		out.At = timestamppb.New(e.At())
	}
	return out
}
