import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LogLevel, OperationKind, OperationState } from '../../gen/mcadmin/v1/common_pb'
import { OperationSchema, WatchOperationResponseSchema } from '../../gen/mcadmin/v1/operation_pb'
import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import type { OperationSource } from './watch'
import { useOperationStream } from './useOperationStream'

function op(overrides: MessageInitShape<typeof OperationSchema> = {}): Operation {
  return create(OperationSchema, {
    id: 'op-1',
    kind: OperationKind.SERVER_RESTART,
    state: OperationState.RUNNING,
    stepIndex: 1,
    stepTotal: 4,
    ...overrides,
  })
}

function event(seq: number, message: string, snapshot: Operation = op()) {
  return create(WatchOperationResponseSchema, {
    seq: BigInt(seq),
    level: LogLevel.INFO,
    message,
    snapshot,
  })
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

/**
 * 1 回の接続で配信されるもの。
 *
 * 終わり方を分けているのが要点。正常に終わる（= 操作が終端に達して
 * サーバーが閉じた）のと、例外で切れるのとでは、続きの扱いが逆になる。
 */
type Batch = {
  events: ReturnType<typeof event>[]
  /** 配信し終えたあとに投げる例外。省略すると正常終了。 */
  breaksWith?: Error
}

/** 呼び出しごとに違う結果を返す購読元を作る。 */
function sourceOf(
  active: Operation | null,
  batches: Batch[],
): OperationSource & { calls: bigint[] } {
  const calls: bigint[] = []
  let index = 0

  return {
    calls,
    active: async () => active,
    watch: (_id, fromSeq) => {
      calls.push(fromSeq)
      const batch = batches[index++] ?? { events: [] }
      return {
        async *[Symbol.asyncIterator]() {
          for (const e of batch.events) {
            yield e
          }
          if (batch.breaksWith) {
            throw batch.breaksWith
          }
        },
      }
    },
  }
}

describe('起動時', () => {
  it('進行中の操作が無ければ何も購読しない', async () => {
    const source = sourceOf(null, [])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.operation).toBeNull())
    expect(source.calls).toEqual([])
  })

  /**
   * 再読み込みの直後でもログが埋まっている必要がある。
   * fromSeq=0 で最初から再送させるのがその仕組み。
   */
  it('進行中の操作を最初から再送させる', async () => {
    const source = sourceOf(op(), [{ events: [event(1, '一'), event(2, '二')] }])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.log).toHaveLength(2))
    expect(source.calls[0]).toBe(0n)
    expect(result.current.log.map((l) => l.message)).toEqual(['一', '二'])
  })
})

describe('再接続', () => {
  it('切断されたら最後の続きから取り直す', async () => {
    const source = sourceOf(op(), [
      {
        events: [event(1, '一'), event(2, '二')],
        breaksWith: new ConnectError('切れました', Code.Unavailable),
      },
      { events: [event(3, '三')] },
    ])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.log).toHaveLength(2))

    // 待ち時間を進めて再接続させる
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000)
    })
    await waitFor(() => expect(result.current.log).toHaveLength(3))

    // 1 本目は最初から、2 本目は受け取った続きから
    expect(source.calls).toEqual([0n, 2n])
  })

  /**
   * 同じイベントが再送されても増えない。再接続の位置指定が
   * 多少ずれていても表示が壊れないようにするための番人。
   */
  it('再送されたイベントでログが二重にならない', async () => {
    const source = sourceOf(op(), [
      {
        events: [event(1, '一'), event(2, '二')],
        breaksWith: new ConnectError('切れました', Code.Unavailable),
      },
      // サーバーが多めに再送してきた場合
      { events: [event(1, '一'), event(2, '二'), event(3, '三')] },
    ])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.log).toHaveLength(2))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000)
    })

    await waitFor(() => expect(result.current.log).toHaveLength(3))
    expect(result.current.log.map((l) => l.message)).toEqual(['一', '二', '三'])
  })

  /**
   * 終端に達したらサーバーがストリームを閉じる。ここで繋ぎ直すと
   * 接続 → 再送 → 切断 を延々と繰り返してサーバーを叩き続ける。
   */
  it('操作が終わったら繋ぎ直さない', async () => {
    const done = op({ state: OperationState.SUCCEEDED })
    const source = sourceOf(op(), [{ events: [event(1, '一', done)] }, { events: [event(2, '二')] }])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.operation?.state).toBe(OperationState.SUCCEEDED))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })

    expect(source.calls).toHaveLength(1)
    expect(result.current.log).toHaveLength(1)
  })
})

describe('後始末', () => {
  /**
   * StrictMode では購読の副作用が 2 回走る。中断できないと
   * ストリームが二重になり、ログが倍に増える。
   */
  it('後始末で購読を中断する', async () => {
    let aborted = false
    const source: OperationSource = {
      active: async () => op(),
      watch: (_id, _seq, signal) => {
        signal.addEventListener('abort', () => {
          aborted = true
        })
        return {
          // 終わらないストリーム
          async *[Symbol.asyncIterator]() {
            yield event(1, '一')
            await new Promise(() => {})
          },
        }
      },
    }

    const { result, unmount } = renderHook(() => useOperationStream(source))
    await waitFor(() => expect(result.current.log).toHaveLength(1))

    unmount()
    expect(aborted).toBe(true)
  })
})

describe('操作の差し込み', () => {
  /**
   * 変更系の応答が返ってきた時点で購読を始める。
   * GetActiveOperation を待つと、押した直後に何も起きない時間ができる。
   */
  it('差し込んだ操作をすぐ購読する', async () => {
    const source = sourceOf(null, [{ events: [event(1, '始めました', op({ id: 'op-2' }))] }])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.operation).toBeNull())

    act(() => {
      result.current.begin(op({ id: 'op-2' }))
    })

    await waitFor(() => expect(result.current.log).toHaveLength(1))
    expect(result.current.operation?.id).toBe('op-2')
  })
})

describe('isBusy', () => {
  /**
   * 失敗した操作は帯に残す。終端で消すと、失敗の理由を伝える
   * 唯一の場所が消えて「何も言わずに操作が消えた」ことになる。
   * 一方でボタンは押せるようにしないと、二度と何もできなくなる。
   */
  it('終端に達したら操作は残るがボタンは押せる', async () => {
    const failed = op({ state: OperationState.FAILED, errorMessage: '失敗しました' })
    const source = sourceOf(op(), [{ events: [event(1, 'だめでした', failed)] }])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.operation?.state).toBe(OperationState.FAILED))
    expect(result.current.isBusy).toBe(false)
    expect(result.current.operation?.errorMessage).toBe('失敗しました')
  })

  it('進行中はボタンを押せない', async () => {
    const source = sourceOf(op(), [
      { events: [event(1, '一')], breaksWith: new ConnectError('保留', Code.Unavailable) },
    ])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.isBusy).toBe(true))
  })

  it('閉じると帯も消える', async () => {
    const done = op({ state: OperationState.SUCCEEDED })
    const source = sourceOf(op(), [{ events: [event(1, '一', done)] }])
    const { result } = renderHook(() => useOperationStream(source))

    await waitFor(() => expect(result.current.operation).not.toBeNull())
    act(() => {
      result.current.dismiss()
    })

    expect(result.current.operation).toBeNull()
  })
})

describe('別の経路で始まった操作', () => {
  /**
   * 操作は別のタブや別の端末からも始まる。起動時の 1 回だけ探すと、
   * そのタブは再読み込みするまで何も起きていないように見え、
   * 変更系のボタンも押せてしまう（サーバー側では弾かれる）。
   */
  it('購読していない間は探し続ける', async () => {
    let current: Operation | null = null
    const calls: bigint[] = []
    const source: OperationSource = {
      active: async () => current,
      watch: (_id, fromSeq) => {
        calls.push(fromSeq)
        return {
          async *[Symbol.asyncIterator]() {
            // サーバーは見つかった操作そのものの snapshot を返す
            yield event(1, 'よそで始まった操作', current ?? op())
            await new Promise(() => {})
          },
        }
      },
    }

    const { result } = renderHook(() => useOperationStream(source))
    await waitFor(() => expect(result.current.operation).toBeNull())

    // このタブの外で操作が始まった
    current = op({ id: 'op-external' })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6_000)
    })

    await waitFor(() => expect(result.current.operation?.id).toBe('op-external'))
    expect(result.current.isBusy).toBe(true)
  })

  // 購読中も探し続けると、同じ操作を何度も差し込んでログが消える。
  it('購読している間は探さない', async () => {
    let lookups = 0
    const source: OperationSource = {
      active: async () => {
        lookups++
        return op()
      },
      watch: () => ({
        async *[Symbol.asyncIterator]() {
          yield event(1, '一')
          await new Promise(() => {})
        },
      }),
    }

    const { result } = renderHook(() => useOperationStream(source))
    await waitFor(() => expect(result.current.log).toHaveLength(1))

    const before = lookups
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })

    expect(lookups).toBe(before)
    expect(result.current.log).toHaveLength(1)
  })
})
