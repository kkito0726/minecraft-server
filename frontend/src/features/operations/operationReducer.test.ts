import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import { LogLevel, OperationKind, OperationState } from '../../gen/mcadmin/v1/common_pb'
import { OperationSchema, WatchOperationResponseSchema } from '../../gen/mcadmin/v1/operation_pb'
import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import { initialState, isTerminal, operationReducer } from './operationReducer'

function op(overrides: MessageInitShape<typeof OperationSchema> = {}): Operation {
  return create(OperationSchema, {
    id: 'op-1',
    kind: OperationKind.SERVER_RESTART,
    state: OperationState.RUNNING,
    stepIndex: 1,
    stepTotal: 4,
    currentStep: 'サーバーを停止しています',
    stepNames: ['停止', '待機', '起動', '確認'],
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

describe('operationReducer', () => {
  it('イベントを受けると状態とログが増える', () => {
    const s = operationReducer(initialState, { type: 'event', event: event(1, '停止しました') })

    expect(s.operation?.id).toBe('op-1')
    expect(s.lastSeq).toBe(1n)
    expect(s.log).toHaveLength(1)
    expect(s.log[0]?.message).toBe('停止しました')
  })

  it('ログは受け取った順に積む', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(1, '一') })
    s = operationReducer(s, { type: 'event', event: event(2, '二') })
    s = operationReducer(s, { type: 'event', event: event(3, '三') })

    expect(s.log.map((l) => l.message)).toEqual(['一', '二', '三'])
    expect(s.lastSeq).toBe(3n)
  })

  /**
   * これがこのフェーズでいちばん重要な不変条件。
   *
   * 同じイベントが 3 つの経路で届く。起動時の fromSeq=0 の再送、
   * 再接続時の fromSeq=lastSeq+1、変更系の応答から差し込む Operation。
   * 落とさないとログが再接続のたびに二重・三重になる。
   */
  it('受信済みの seq は捨てる', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(1, '一') })
    s = operationReducer(s, { type: 'event', event: event(2, '二') })

    // 再接続で 1 と 2 がもう一度届いた
    s = operationReducer(s, { type: 'event', event: event(1, '一') })
    s = operationReducer(s, { type: 'event', event: event(2, '二') })
    s = operationReducer(s, { type: 'event', event: event(3, '三') })

    expect(s.log.map((l) => l.message)).toEqual(['一', '二', '三'])
    expect(s.lastSeq).toBe(3n)
  })

  it('捨てるときは状態オブジェクトごと据え置く', () => {
    const s1 = operationReducer(initialState, { type: 'event', event: event(2, '二') })
    const s2 = operationReducer(s1, { type: 'event', event: event(1, '一') })

    expect(s2).toBe(s1)
  })

  /**
   * snapshot は毎回まるごと置き換える。イベントを取りこぼしても
   * 次の 1 件で表示が正しい状態に戻る。
   */
  it('snapshot は差分ではなく置き換え', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(1, '一', op()) })
    s = operationReducer(s, {
      type: 'event',
      event: event(2, '二', op({ stepIndex: 3, currentStep: 'サーバーを起動しています' })),
    })

    expect(s.operation?.stepIndex).toBe(3)
    expect(s.operation?.currentStep).toBe('サーバーを起動しています')
  })

  it('snapshot が無いイベントでも直前の状態を失わない', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(1, '一') })
    const withoutSnapshot = create(WatchOperationResponseSchema, {
      seq: 2n,
      level: LogLevel.WARN,
      message: 'snapshot なし',
    })
    s = operationReducer(s, { type: 'event', event: withoutSnapshot })

    expect(s.operation?.id).toBe('op-1')
    expect(s.log).toHaveLength(2)
  })

  it('入力の状態を書き換えない', () => {
    const before = operationReducer(initialState, { type: 'event', event: event(1, '一') })
    const snapshotOfLog = [...before.log]

    operationReducer(before, { type: 'event', event: event(2, '二') })

    expect(before.log).toEqual(snapshotOfLog)
    expect(before.lastSeq).toBe(1n)
  })
})

describe('begin', () => {
  /**
   * 変更系の応答から差し込む Operation は「別の操作の始まり」であって
   * 進行中の操作のイベントではない。ログと seq を 0 に戻す。
   */
  it('新しい操作としてログと seq を初期化する', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(5, '前の操作') })
    s = operationReducer(s, { type: 'begin', operation: op({ id: 'op-2', stepIndex: 0 }) })

    expect(s.operation?.id).toBe('op-2')
    expect(s.log).toEqual([])
    expect(s.lastSeq).toBe(0n)
  })

  /**
   * 同じ操作を二度差し込むとログが消える。定期的な探索と変更系の応答が
   * どちらも begin を呼ぶため、サーバーが操作を作ってから応答が返るまでの
   * 間に探索が当たると起きる。しかも setWatching は同じ値なので購読が
   * 張り直されず、再送でログが埋め直されることもない。
   */
  it('同じ操作を二度差し込んでもログを消さない', () => {
    let s = operationReducer(initialState, { type: 'begin', operation: op({ id: 'op-2' }) })
    s = operationReducer(s, { type: 'event', event: event(1, '一', op({ id: 'op-2' })) })
    s = operationReducer(s, { type: 'event', event: event(2, '二', op({ id: 'op-2' })) })

    const before = s
    s = operationReducer(s, { type: 'begin', operation: op({ id: 'op-2' }) })

    expect(s).toBe(before)
    expect(s.log).toHaveLength(2)
    expect(s.lastSeq).toBe(2n)
  })

  // 応答に含まれる snapshot は PENDING。取り込むと表示が巻き戻る。
  it('二度目の差し込みで状態を巻き戻さない', () => {
    let s = operationReducer(initialState, { type: 'begin', operation: op({ id: 'op-2' }) })
    s = operationReducer(s, {
      type: 'event',
      event: event(1, '一', op({ id: 'op-2', state: OperationState.RUNNING, stepIndex: 3 })),
    })
    s = operationReducer(s, {
      type: 'begin',
      operation: op({ id: 'op-2', state: OperationState.PENDING, stepIndex: 0 }),
    })

    expect(s.operation?.state).toBe(OperationState.RUNNING)
    expect(s.operation?.stepIndex).toBe(3)
  })

  it('直後のイベントを seq=1 から受け取れる', () => {
    let s = operationReducer(initialState, { type: 'begin', operation: op({ id: 'op-2' }) })
    s = operationReducer(s, {
      type: 'event',
      event: event(1, '始めました', op({ id: 'op-2' })),
    })

    expect(s.log).toHaveLength(1)
    expect(s.lastSeq).toBe(1n)
  })
})

describe('dismiss', () => {
  // 終わった操作の帯は利用者が閉じる。自動で消すと失敗の理由が読めない。
  it('操作を消す', () => {
    let s = operationReducer(initialState, { type: 'event', event: event(1, '一') })
    s = operationReducer(s, { type: 'dismiss' })

    expect(s.operation).toBeNull()
    expect(s.log).toEqual([])
  })
})

describe('isTerminal', () => {
  it.each([
    [OperationState.PENDING, false],
    [OperationState.RUNNING, false],
    [OperationState.SUCCEEDED, true],
    [OperationState.FAILED, true],
    [OperationState.UNSPECIFIED, false],
  ])('%s → %s', (state, want) => {
    expect(isTerminal(op({ state }))).toBe(want)
  })

  it('操作が無ければ終端ではない', () => {
    expect(isTerminal(null)).toBe(false)
  })
})
