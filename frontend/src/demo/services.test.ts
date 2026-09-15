import { ConnectError } from '@connectrpc/connect'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BackupMode, RestoreTarget, VersionVerdict } from '../gen/mcadmin/v1/backup_pb'
import { ContainerState } from '../gen/mcadmin/v1/server_pb'
import { backupImpl } from './backupService'
import { resetOperations } from './operations'
import { serverImpl } from './serverService'
import { getState, resetState } from './state'
import { worldImpl } from './worldService'

/**
 * 実装は Connect のハンドラとして呼ばれる。試験では引数の型だけ合わせて
 * 直接呼ぶ。第 2 引数（HandlerContext）はここで使う実装が触らない。
 */
type Call<T> = (req: T, ctx: never) => unknown

function call<T>(impl: unknown, req: T): unknown {
  return (impl as Call<T>)(req, undefined as never)
}

beforeEach(() => {
  vi.useFakeTimers()
  resetState()
  resetOperations()
})

afterEach(() => {
  resetOperations()
  vi.useRealTimers()
})

async function runToEnd() {
  await vi.advanceTimersByTimeAsync(20_000)
}

describe('ServerService', () => {
  it('停止すると状態と人数が落ちる', async () => {
    call(serverImpl.stopServer, {})
    await runToEnd()

    const status = call(serverImpl.getStatus, {}) as { containerState: ContainerState }
    expect(status.containerState).toBe(ContainerState.EXITED)
    expect(getState().onlinePlayers).toBe(0)
  })

  it('公開してよい設定だけを返す', () => {
    const config = call(serverImpl.getConfig, {}) as { values: Record<string, string> }

    expect(config.values.MC_LEVEL).toBe('world')
    expect(Object.keys(config.values)).not.toContain('ADMIN_TOKEN')
    expect(Object.keys(config.values)).not.toContain('RCON_PASSWORD')
  })
})

describe('WorldService', () => {
  it('切り替えると稼働中のワールドが移る', async () => {
    call(worldImpl.switchWorld, { name: 'creative' })
    await runToEnd()

    expect(getState().activeLevel).toBe('creative')
  })

  it('複製すると一覧が増え、元のワールドは残る', async () => {
    call(worldImpl.cloneWorld, { source: 'world', destination: 'world-copy' })
    await runToEnd()

    const names = getState().worlds.map((w) => w.name)
    expect(names).toContain('world')
    expect(names).toContain('world-copy')
  })

  // 取り消せない操作の関門。デモでも素通りさせない。
  it('名前が一致しなければ削除できない', () => {
    expect(() => call(worldImpl.deleteWorld, { name: 'creative', confirmName: 'crea' })).toThrow(
      ConnectError,
    )
    expect(getState().worlds.map((w) => w.name)).toContain('creative')
  })

  it('稼働中のワールドは削除できない', () => {
    expect(() => call(worldImpl.deleteWorld, { name: 'world', confirmName: 'world' })).toThrow(
      ConnectError,
    )
  })

  it('削除すると退避に移る', async () => {
    call(worldImpl.deleteWorld, { name: 'creative', confirmName: 'creative' })
    await runToEnd()

    expect(getState().worlds.map((w) => w.name)).not.toContain('creative')
    expect(getState().quarantines[0]?.originalLevel).toBe('creative')
  })

  it('退避は完全に削除できる', () => {
    const target = getState().quarantines[0]!
    call(worldImpl.purgeQuarantine, { name: target.name })

    expect(getState().quarantines).toHaveLength(0)
  })
})

describe('BackupService', () => {
  it('取得すると一覧の先頭に増える', async () => {
    const before = getState().backups.length
    call(backupImpl.createBackup, { mode: BackupMode.HOT, note: 'before update' })
    await runToEnd()

    const backups = getState().backups
    expect(backups).toHaveLength(before + 1)
    // メモは使える文字だけがファイル名に残る。
    expect(backups[0]?.id).toContain('beforeupdate')
  })

  it('版が一致していれば承諾は要らない', () => {
    const target = getState().backups[0]!
    const result = call(backupImpl.preflightRestore, { backupId: target.id }) as {
      verdict: VersionVerdict
      requiresConfirmation: boolean
    }

    expect(result.verdict).toBe(VersionVerdict.MATCH)
    expect(result.requiresConfirmation).toBe(false)
  })

  it('版が古く名前も違えば、理由を添えて承諾を求める', () => {
    const old = getState().backups.find((b) => b.archiveLevel === 'hardcore-2025')!
    const result = call(backupImpl.preflightRestore, { backupId: old.id }) as {
      verdict: VersionVerdict
      requiresConfirmation: boolean
      warnings: string[]
    }

    expect(result.verdict).toBe(VersionVerdict.OLDER_WILL_UPGRADE)
    expect(result.requiresConfirmation).toBe(true)
    expect(result.warnings.join()).toContain('稼働しているのは "world"')
  })

  it('承諾なしでは復元できない', () => {
    const old = getState().backups.find((b) => b.archiveLevel === 'hardcore-2025')!

    expect(() =>
      call(backupImpl.restoreBackup, {
        backupId: old.id,
        target: RestoreTarget.ARCHIVE_LEVEL,
        acknowledgeVersionWarning: false,
        confirmLevelName: 'hardcore-2025',
      }),
    ).toThrow(ConnectError)
  })

  it('名前が一致しなければ復元できない', () => {
    const target = getState().backups[0]!

    expect(() =>
      call(backupImpl.restoreBackup, {
        backupId: target.id,
        target: RestoreTarget.ARCHIVE_LEVEL,
        acknowledgeVersionWarning: true,
        confirmLevelName: 'typo',
      }),
    ).toThrow(ConnectError)
  })

  it('復元すると現在のワールドが退避される', async () => {
    const target = getState().backups[0]!
    call(backupImpl.restoreBackup, {
      backupId: target.id,
      target: RestoreTarget.ARCHIVE_LEVEL,
      acknowledgeVersionWarning: false,
      confirmLevelName: 'world',
    })
    await runToEnd()

    expect(getState().quarantines.some((q) => q.fromRestore)).toBe(true)
  })

  it('存在しないバックアップは見つからないと答える', () => {
    expect(() => call(backupImpl.preflightRestore, { backupId: 'no-such.zip' })).toThrow(
      ConnectError,
    )
  })

  it('0 世代は受け付けない', () => {
    expect(() =>
      call(backupImpl.setRetentionPolicy, { policy: { keepCount: 0, keepDays: 0 } }),
    ).toThrow(ConnectError)
  })

  it('保持ポリシーは保存できる', () => {
    call(backupImpl.setRetentionPolicy, { policy: { keepCount: 3, keepDays: 7 } })

    expect(getState().retention).toEqual({ keepCount: 3, keepDays: 7 })
  })

  // 世代数と日数の両方を満たさないものだけを消し、最新の 1 世代は必ず残す。
  it('確認しただけでは消えない', () => {
    call(backupImpl.setRetentionPolicy, { policy: { keepCount: 1, keepDays: 1 } })
    const before = getState().backups.length

    const preview = call(backupImpl.pruneBackups, { dryRun: true }) as { deletedIds: string[] }
    expect(preview.deletedIds.length).toBeGreaterThan(0)
    expect(getState().backups).toHaveLength(before)

    call(backupImpl.pruneBackups, { dryRun: false })
    expect(getState().backups.length).toBeLessThan(before)
    expect(getState().backups.length).toBeGreaterThan(0)
  })

  it('削除すると一覧から消える', () => {
    const target = getState().backups[0]!
    call(backupImpl.deleteBackup, { backupId: target.id })

    expect(getState().backups.map((b) => b.id)).not.toContain(target.id)
  })
})
