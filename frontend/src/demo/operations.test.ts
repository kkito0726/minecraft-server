import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LogLevel, OperationState } from '../gen/mcadmin/v1/common_pb'
import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import {
  DemoBusyError,
  getActiveOperation,
  isBusy,
  listOperations,
  resetOperations,
  startOperation,
  watchOperation,
} from './operations'

const STEPS = [{ name: '止めています' }, { name: '固めています', withBytes: true }]

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  resetOperations()
  vi.useRealTimers()
})

/** 手順が終わりきるまで時間を進める。 */
async function runToEnd() {
  await vi.advanceTimersByTimeAsync(20_000)
}

describe('startOperation', () => {
  it('開始直後は待機中で、手順の名前がそろっている', () => {
    const op = startOperation({ kind: OperationKind.BACKUP_CREATE, steps: STEPS })

    expect(op.state).toBe(OperationState.PENDING)
    expect(op.stepTotal).toBe(2)
    expect(op.stepNames).toEqual(['止めています', '固めています'])
  })

  // 実物はロックで排他する。同時に 2 つ走らせない約束はデモでも守る。
  it('実行中は次の操作を断る', () => {
    startOperation({ kind: OperationKind.BACKUP_CREATE, steps: STEPS })

    expect(isBusy()).toBe(true)
    expect(() => startOperation({ kind: OperationKind.SERVER_STOP, steps: STEPS })).toThrow(
      DemoBusyError,
    )
  })

  it('終われば成功になり、次を始められる', async () => {
    const commit = vi.fn()
    startOperation({ kind: OperationKind.SERVER_START, steps: STEPS, commit })
    await runToEnd()

    expect(commit).toHaveBeenCalledTimes(1)
    expect(isBusy()).toBe(false)
    expect(listOperations(10)[0]?.state).toBe(OperationState.SUCCEEDED)
  })

  it('失敗する手順を指定すると、そこで止まって理由が残る', async () => {
    const commit = vi.fn()
    startOperation({
      kind: OperationKind.BACKUP_RESTORE,
      steps: STEPS,
      commit,
      failAtStep: 1,
      failMessage: '展開に失敗しました',
    })
    await runToEnd()

    const op = listOperations(10)[0]
    expect(op?.state).toBe(OperationState.FAILED)
    expect(op?.errorMessage).toBe('展開に失敗しました')
    // 失敗したら状態は変えない。
    expect(commit).not.toHaveBeenCalled()
    expect(isBusy()).toBe(false)
  })

  it('バイト数の手順では進み具合が増えていく', async () => {
    startOperation({
      kind: OperationKind.BACKUP_CREATE,
      steps: STEPS,
      totalBytes: 1_000n,
    })
    await vi.advanceTimersByTimeAsync(1_500)
    const midway = getActiveOperation()

    expect(midway?.bytesTotal).toBe(1_000n)
    expect(midway?.bytesDone).toBeGreaterThan(0n)
  })
})

describe('watchOperation', () => {
  it('記録済みのイベントを最初から再送してから終わる', async () => {
    const op = startOperation({ kind: OperationKind.SERVER_RESTART, steps: STEPS })
    await runToEnd()

    const seen: string[] = []
    for await (const event of watchOperation(op.id, 0n)) {
      seen.push(event.message)
    }

    expect(seen.length).toBeGreaterThan(2)
    expect(seen.at(-1)).toBe('完了しました')
  })

  // 再接続は「最後に受け取った次」から取り直す。ログが二重にならないための約束。
  it('fromSeq を指定するとその位置から配信する', async () => {
    const op = startOperation({ kind: OperationKind.SERVER_RESTART, steps: STEPS })
    await runToEnd()

    const all: bigint[] = []
    for await (const event of watchOperation(op.id, 0n)) {
      all.push(event.seq)
    }
    const rest: bigint[] = []
    for await (const event of watchOperation(op.id, 3n)) {
      rest.push(event.seq)
    }

    expect(all[0]).toBe(1n)
    expect(rest[0]).toBe(3n)
    expect(rest.length).toBeLessThan(all.length)
  })

  it('警告の手順は警告として流す', async () => {
    const op = startOperation({
      kind: OperationKind.WORLD_DELETE,
      steps: [{ name: '退避しています', warn: true }],
    })
    await runToEnd()

    const levels: LogLevel[] = []
    for await (const event of watchOperation(op.id, 0n)) {
      levels.push(event.level)
    }

    expect(levels).toContain(LogLevel.WARN)
  })
})
