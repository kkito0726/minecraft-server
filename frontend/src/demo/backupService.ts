/**
 * BackupService のデモ実装。
 *
 * 事前確認（PreflightRestore）の判定は実物と同じ考え方で組む。
 * level.dat の DataVersion どうしを比べ、一致しなければ承諾を求める。
 * ここを緩めると、デモを見た人が「復元は軽い操作だ」と誤解する。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import {
  BackupMode,
  BackupService,
  RestoreTarget,
  VersionVerdict,
} from '../gen/mcadmin/v1/backup_pb'
import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import { toBackup, toOperation, toVersion } from './messages'
import { startOperation } from './operations'
import { busyGuard, failedPrecondition, invalidArgument, notFound } from './rpcErrors'
import { activeWorld, findBackup, getState, sortedBackups, updateState } from './state'
import type { DemoBackup, DemoState, DemoWorld } from './state'
import { COLD_BACKUP_STEPS, HOT_BACKUP_STEPS, RESTORE_STEPS } from './steps'

/** デモの空き容量。復元の必要量と比べるために固定で持つ。 */
const AVAILABLE_BYTES = 18_400_000_000n

function stamp(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}`
}

function verdictOf(state: DemoState, backup: DemoBackup): VersionVerdict {
  const current = activeWorld(state)?.version
  if (!current?.readable || !backup.version.readable) {
    return VersionVerdict.UNKNOWN
  }
  if (current.dataVersion === backup.version.dataVersion) {
    return VersionVerdict.MATCH
  }
  return backup.version.dataVersion < current.dataVersion
    ? VersionVerdict.OLDER_WILL_UPGRADE
    : VersionVerdict.NEWER_INCOMPATIBLE
}

/** 警告文はサーバーが組み立てる、という約束もそのまま再現する。 */
function warningsOf(state: DemoState, backup: DemoBackup, verdict: VersionVerdict): string[] {
  const warnings: string[] = []

  if (backup.archiveLevel !== state.activeLevel) {
    warnings.push(
      `アーカイブのワールドは "${backup.archiveLevel}" ですが、現在稼働しているのは "${state.activeLevel}" です。そのまま復元すると稼働中のワールドは変化しません。`,
    )
  }
  if (verdict === VersionVerdict.OLDER_WILL_UPGRADE) {
    warnings.push(
      'アーカイブの方が古いバージョンです。復元すると、開いたときに片道のアップグレードが再度走ります。',
    )
  }
  if (verdict === VersionVerdict.NEWER_INCOMPATIBLE) {
    warnings.push(
      'アーカイブの方が新しいバージョンです。現在の MC_VERSION では開けない可能性があります。',
    )
  }
  if (verdict === VersionVerdict.UNKNOWN) {
    warnings.push('どちらかの level.dat を読めませんでした。安全のため承諾を求めます。')
  }
  return warnings
}

function requireBackup(id: string): DemoBackup {
  const backup = findBackup(getState(), id)
  if (!backup) {
    throw notFound(`バックアップが見つかりません: ${id}`)
  }
  return backup
}

/** 保持ポリシーの適用対象。最も新しい 1 世代は設定に関わらず必ず残す。 */
function victims(state: DemoState): DemoBackup[] {
  const sorted = sortedBackups(state.backups)
  const limit = Date.now() - state.retention.keepDays * 24 * 60 * 60 * 1000

  return sorted.filter((backup, index) => {
    if (index === 0) {
      return false
    }
    const overCount = index >= state.retention.keepCount
    const tooOld = state.retention.keepDays > 0 && backup.createdAt.getTime() < limit
    // 世代数と日数の両方を満たさないものだけを消す。
    return overCount && tooOld
  })
}

/**
 * 復元の結果を状態に反映する。
 *
 * 退避するのは「展開先と同じ名前のワールド」であって、稼働中のものではない。
 * 復元先にアーカイブのワールド名を選んだ場合、いま遊んでいるワールドは
 * 1 バイトも動かない。上書きではなく「退避してから展開」なので、
 * うまくいかなければ退避から戻せる。
 */
function applyRestore(state: DemoState, backup: DemoBackup, level: string): DemoState {
  const replaced = state.worlds.find((w) => w.name === level)
  const restored: DemoWorld = {
    name: level,
    sizeBytes: backup.sizeBytes * 2n,
    lastPlayed: new Date(),
    version: { ...backup.version, levelName: level },
    hasSessionLock: true,
  }

  return {
    ...state,
    activeLevel: level,
    quarantines: replaced
      ? [
          {
            name: `${replaced.name}.broken-${stamp(new Date())}`,
            originalLevel: replaced.name,
            quarantinedAt: new Date(),
            sizeBytes: replaced.sizeBytes,
            fromRestore: true,
          },
          ...state.quarantines,
        ]
      : state.quarantines,
    worlds: [...state.worlds.filter((w) => w.name !== level), restored],
  }
}

export const backupImpl: Partial<ServiceImpl<typeof BackupService>> = {
  listBackups: () => {
    const state = getState()
    return {
      backups: sortedBackups(state.backups).map(toBackup),
      directory: state.backupDir,
      totalSizeBytes: state.backups.reduce((sum, b) => sum + b.sizeBytes, 0n),
    }
  },

  createBackup: (req) => {
    const state = getState()
    const cold = req.mode === BackupMode.COLD
    const world = activeWorld(state)
    const size = (world?.sizeBytes ?? 200_000_000n) / 2n
    const note = req.note.replace(/[^A-Za-z0-9_]/g, '').slice(0, 32)

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.BACKUP_CREATE,
          steps: cold ? COLD_BACKUP_STEPS : HOT_BACKUP_STEPS,
          totalBytes: size,
          attributes: { mode: cold ? 'cold' : 'hot' },
          commit: () => {
            const now = new Date()
            const suffix = note ? `-${note}` : ''
            const id = `backup-${state.configuredVersion}-${state.activeLevel}-${stamp(now)}${suffix}.zip`
            updateState((s) => ({
              ...s,
              backups: sortedBackups([
                {
                  id,
                  sizeBytes: size,
                  createdAt: now,
                  declaredVersion: s.configuredVersion,
                  archiveLevel: s.activeLevel,
                  version: activeWorld(s)?.version ?? {
                    readable: false,
                    name: '',
                    dataVersion: 0,
                    levelName: '',
                  },
                  entryRoots: [`data/${s.activeLevel}`],
                },
                ...s.backups,
              ]),
            }))
          },
        }),
      ),
    }))
  },

  deleteBackup: (req) => {
    const backup = requireBackup(req.backupId)
    updateState((s) => ({ ...s, backups: s.backups.filter((b) => b.id !== req.backupId) }))
    return { freedBytes: backup.sizeBytes }
  },

  preflightRestore: (req) => {
    const state = getState()
    const backup = requireBackup(req.backupId)
    const verdict = verdictOf(state, backup)
    const mismatch = backup.archiveLevel !== state.activeLevel

    return {
      backup: toBackup(backup),
      currentLevel: state.activeLevel,
      currentWorldVersion: toVersion(activeWorld(state)?.version),
      configuredMcVersion: state.configuredVersion,
      verdict,
      levelNameMismatch: mismatch,
      warnings: warningsOf(state, backup, verdict),
      requiresConfirmation: verdict !== VersionVerdict.MATCH || mismatch,
      requiredBytes: backup.sizeBytes * 2n,
      availableBytes: AVAILABLE_BYTES,
    }
  },

  restoreBackup: (req) => {
    const state = getState()
    const backup = requireBackup(req.backupId)
    const toArchive = req.target !== RestoreTarget.CURRENT_LEVEL
    const level = toArchive ? backup.archiveLevel : state.activeLevel
    const verdict = verdictOf(state, backup)
    const needsAck = verdict !== VersionVerdict.MATCH || backup.archiveLevel !== state.activeLevel

    // 関門は 2 つ。名前の完全一致と、警告への承諾。
    if (req.confirmLevelName !== level) {
      throw invalidArgument('復元先のワールド名が一致していません')
    }
    if (needsAck && !req.acknowledgeVersionWarning) {
      throw failedPrecondition('警告への承諾が必要です')
    }

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.BACKUP_RESTORE,
          steps: RESTORE_STEPS,
          totalBytes: backup.sizeBytes,
          attributes: { backup_id: backup.id },
          commit: () => updateState((s) => applyRestore(s, backup, level)),
        }),
      ),
    }))
  },

  getRetentionPolicy: () => ({ policy: getState().retention }),

  setRetentionPolicy: (req) => {
    const keepCount = req.policy?.keepCount ?? 0
    const keepDays = req.policy?.keepDays ?? 0
    // 0 世代はバックアップの全損を意味する。実物と同じくここで断る。
    if (keepCount < 1) {
      throw invalidArgument('残す世代数は 1 以上にしてください')
    }

    const next = { keepCount, keepDays }
    updateState((s) => ({ ...s, retention: next }))
    return { policy: next }
  },

  pruneBackups: (req) => {
    const targets = victims(getState())
    const freed = targets.reduce((sum, b) => sum + b.sizeBytes, 0n)

    if (!req.dryRun && targets.length > 0) {
      const ids = new Set(targets.map((b) => b.id))
      updateState((s) => ({ ...s, backups: s.backups.filter((b) => !ids.has(b.id)) }))
    }
    return { deletedIds: targets.map((b) => b.id), freedBytes: freed }
  },
}
