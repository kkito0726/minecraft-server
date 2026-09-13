/**
 * デモの状態を proto のメッセージに変換する。
 *
 * 変換をここに閉じ込めておくと、状態の遷移（state.ts）と手順の進行
 * （operations.ts）は proto を知らないままでいられる。試験もそちらだけで足りる。
 *
 * 省略可能なメッセージ欄には undefined を明示的に渡さない。
 * exactOptionalPropertyTypes が有効なので、渡すかどうかごと分岐させる。
 */
import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'

import { BackupSchema } from '../gen/mcadmin/v1/backup_pb'
import { WorldVersionSchema } from '../gen/mcadmin/v1/common_pb'
import { OperationSchema, WatchOperationResponseSchema } from '../gen/mcadmin/v1/operation_pb'
import { ContainerState, GetStatusResponseSchema, SavingState } from '../gen/mcadmin/v1/server_pb'
import { QuarantineSchema, WorldSchema } from '../gen/mcadmin/v1/world_pb'
import { getActiveOperation } from './operations'
import type { DemoLogEvent, DemoOperation } from './operations'
import { activeWorld } from './state'
import type { DemoBackup, DemoQuarantine, DemoState, DemoVersion, DemoWorld } from './state'

export function toVersion(version: DemoVersion | undefined) {
  if (!version) {
    return create(WorldVersionSchema, { readable: false })
  }
  return create(WorldVersionSchema, {
    readable: version.readable,
    name: version.name,
    dataVersion: version.dataVersion,
    levelName: version.levelName,
  })
}

export function toWorld(state: DemoState, world: DemoWorld) {
  return create(WorldSchema, {
    name: world.name,
    active: state.activeLevel === world.name,
    sizeBytes: world.sizeBytes,
    lastPlayed: timestampFromDate(world.lastPlayed),
    version: toVersion(world.version),
    hasSessionLock: world.hasSessionLock,
  })
}

export function toQuarantine(quarantine: DemoQuarantine) {
  return create(QuarantineSchema, {
    name: quarantine.name,
    originalLevel: quarantine.originalLevel,
    quarantinedAt: timestampFromDate(quarantine.quarantinedAt),
    sizeBytes: quarantine.sizeBytes,
    fromRestore: quarantine.fromRestore,
  })
}

export function toBackup(backup: DemoBackup) {
  return create(BackupSchema, {
    id: backup.id,
    sizeBytes: backup.sizeBytes,
    createdAt: timestampFromDate(backup.createdAt),
    declaredVersion: backup.declaredVersion,
    archiveLevel: backup.archiveLevel,
    version: toVersion(backup.version),
    entryRoots: backup.entryRoots,
  })
}

/**
 * サーバーの状態。
 *
 * 停止中は人数を 0 にする。止まっているのに人が繋がっている表示は、
 * 実物では起こりえない。
 */
export function toStatus(state: DemoState) {
  const active = getActiveOperation()

  return create(GetStatusResponseSchema, {
    containerState: state.running ? ContainerState.RUNNING : ContainerState.EXITED,
    healthy: state.running && state.healthy,
    ...(state.startedAt ? { containerStartedAt: timestampFromDate(state.startedAt) } : {}),
    configuredVersion: state.configuredVersion,
    activeLevel: state.activeLevel,
    activeWorldVersion: toVersion(activeWorld(state)?.version),
    onlinePlayers: state.running ? state.onlinePlayers : 0,
    maxPlayers: state.maxPlayers,
    savingState: SavingState.ASSUMED_ON,
    interruptedOperationDetected: false,
    ...(active ? { activeOperation: toOperation(active) } : {}),
  })
}

export function toOperation(operation: DemoOperation) {
  return create(OperationSchema, {
    id: operation.id,
    kind: operation.kind,
    state: operation.state,
    startedAt: timestampFromDate(operation.startedAt),
    ...(operation.finishedAt ? { finishedAt: timestampFromDate(operation.finishedAt) } : {}),
    stepIndex: operation.stepIndex,
    stepTotal: operation.stepTotal,
    currentStep: operation.currentStep,
    stepNames: operation.stepNames,
    bytesDone: operation.bytesDone,
    bytesTotal: operation.bytesTotal,
    errorCode: operation.errorCode,
    errorMessage: operation.errorMessage,
    attributes: operation.attributes,
  })
}

export function toEvent(event: DemoLogEvent) {
  return create(WatchOperationResponseSchema, {
    seq: event.seq,
    at: timestampFromDate(event.at),
    level: event.level,
    message: event.message,
    snapshot: toOperation(event.snapshot),
  })
}
