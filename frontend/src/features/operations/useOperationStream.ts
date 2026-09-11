import { Code, ConnectError } from '@connectrpc/connect'
import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import type { Dispatch } from 'react'

import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import { initialState, isTerminal, operationReducer } from './operationReducer'
import type { LogLine, OperationAction } from './operationReducer'
import type { OperationSource } from './watch'

/** 再接続の待ち時間。1 秒から倍々にし、上限で頭打ちにする。 */
const BACKOFF_START_MS = 1_000
const BACKOFF_MAX_MS = 15_000

/**
 * 進行中の操作を探し直す間隔。
 *
 * 操作は別の経路でも始まる。別のタブ、別の端末、あるいは systemd が
 * 起動時に走らせた回復処理。購読していない間だけ定期的に確かめる。
 * これが無いと、そのタブは再読み込みするまで何も起きていないように
 * 見え、変更系のボタンも押せてしまう（サーバー側では弾かれる）。
 */
const DISCOVERY_INTERVAL_MS = 5_000

export type OperationStream = {
  operation: Operation | null
  log: LogLine[]
  /** 進行中かどうか。変更系のボタンの活性を決める。 */
  isBusy: boolean
  /** 変更系の応答から新しい操作を差し込む。 */
  begin: (operation: Operation) => void
  /** 終わった操作の帯を閉じる。 */
  dismiss: () => void
}

/**
 * 進行中の操作を購読し続ける。
 *
 * 起動時に GetActiveOperation で現在の操作を探し、fromSeq=0 で
 * 最初から再送させる。これが無いと、再読み込みの直後の画面が
 * スピナーだけになる。
 */
export function useOperationStream(source: OperationSource): OperationStream {
  const [state, dispatch] = useReducer(operationReducer, initialState)
  // 購読すべき操作の識別子。null なら購読しない。
  const [watching, setWatching] = useState<string | null>(null)
  // 再接続の位置は ref で持つ。state に入れると、更新のたびに
  // 購読の副作用が張り直されてストリームが切れる。
  const lastSeq = useRef(0n)
  lastSeq.current = state.lastSeq
  useDiscovery(source, watching, dispatch, setWatching)
  useSubscription(source, watching, lastSeq, dispatch)

  const begin = useCallback((operation: Operation) => {
    dispatch({ type: 'begin', operation })
    setWatching(operation.id)
  }, [])

  const dismiss = useCallback(() => {
    dispatch({ type: 'dismiss' })
    setWatching(null)
  }, [])

  return {
    operation: state.operation,
    log: state.log,
    isBusy: state.operation !== null && !isTerminal(state.operation),
    begin,
    dismiss,
  }
}

/**
 * 進行中の操作を探す。購読していない間だけ、繰り返し確かめる。
 *
 * 起動時の 1 回だけにすると、このタブ以外で始まった操作に
 * 気づけない。逆に購読中も探し続けると、同じ操作を何度も
 * 差し込んでログが消える。
 */
function useDiscovery(
  source: OperationSource,
  watching: string | null,
  dispatch: Dispatch<OperationAction>,
  setWatching: Dispatch<string | null>,
) {
  useEffect(() => {
    if (watching) {
      return
    }
    let cancelled = false

    const look = () => {
      void source
        .active()
        .then((operation) => {
          if (cancelled || !operation) {
            return
          }
          dispatch({ type: 'begin', operation })
          setWatching(operation.id)
        })
        // 探せなくても画面は開ける。変更系を実行すればその応答から
        // 購読が始まるので、ここで諦めても行き止まりにならない。
        .catch(() => undefined)
    }

    look()
    const timer = setInterval(look, DISCOVERY_INTERVAL_MS)

    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [source, watching, dispatch, setWatching])
}

/**
 * ストリームに繋ぎ、切れたら繋ぎ直す。
 *
 * StrictMode では副作用が 2 回走るため、中断できないとストリームが
 * 二重になってログが倍に増える。
 */
function useSubscription(
  source: OperationSource,
  operationId: string | null,
  lastSeq: { current: bigint },
  dispatch: Dispatch<OperationAction>,
) {
  useEffect(() => {
    if (!operationId) {
      return
    }

    const controller = new AbortController()
    const timers = new Set<ReturnType<typeof setTimeout>>()

    void follow({
      source,
      operationId,
      lastSeq,
      dispatch,
      signal: controller.signal,
      timers,
    })

    return () => {
      controller.abort()
      // 後始末で消さないと、画面から外れた後にタイマーが起きる。
      for (const timer of timers) {
        clearTimeout(timer)
      }
    }
  }, [source, operationId, lastSeq, dispatch])
}

/**
 * 切れるまで読み、切れたら待って続きから読み直す。
 *
 * 終わり方が 4 通りあり、それぞれ扱いが違う。
 *
 *  1. 反復子が正常に終わり、**操作も終端に達している** → サーバーが
 *     終わったから閉じた。繋ぎ直さない。ここで繋ぎ直すと、接続 →
 *     再送 → 切断 を延々と繰り返してサーバーを叩き続ける。
 *  2. 反復子が正常に終わったが、操作はまだ終端に達していない →
 *     終わったから閉じたのではない。**続きから取り直す。**
 *     サーバーは購読者ごとのバッファが溢れると最後のイベントを
 *     送れないまま閉じる。展開の進捗はファイル 1 つごとに出るので、
 *     ファイル数の多いワールドでは現実に起きる。ここで諦めると、
 *     操作は終わっているのに画面が実行中のまま固まり、変更系の
 *     ボタンが全部押せなくなる。再読み込みするまで戻らない。
 *  3. 自分の中断による Canceled → 画面から外れた。何もしない。
 *  4. 操作が見つからない → 履歴からも消えた。追いかけようがない。
 *  5. それ以外の例外 → 本当に切れた。待ってから続きを取り直す。
 */
async function follow(args: {
  source: OperationSource
  operationId: string
  lastSeq: { current: bigint }
  dispatch: Dispatch<OperationAction>
  signal: AbortSignal
  timers: Set<ReturnType<typeof setTimeout>>
}): Promise<void> {
  const { source, operationId, lastSeq, dispatch, signal, timers } = args
  let delay = BACKOFF_START_MS

  while (!signal.aborted) {
    try {
      // 終端に達したかは、受け取ったイベントそのものから判断する。
      // 描画後の state を見ると、最後のイベントを処理した直後には
      // まだ更新されておらず、終わっているのに繋ぎ直してしまう。
      let sawTerminal = false
      for await (const event of source.watch(operationId, lastSeq.current, signal)) {
        if (signal.aborted) {
          return
        }
        dispatch({ type: 'event', event })
        sawTerminal = event.snapshot !== undefined && isTerminal(event.snapshot)
      }
      if (sawTerminal) {
        return
      }
    } catch (err) {
      if (signal.aborted || isCanceled(err)) {
        return
      }
      if (isNotFound(err)) {
        return
      }
    }

    await sleep(delay, timers)
    delay = Math.min(delay * 2, BACKOFF_MAX_MS)
  }
}

function sleep(ms: number, timers: Set<ReturnType<typeof setTimeout>>): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => {
      timers.delete(timer)
      resolve()
    }, ms)
    timers.add(timer)
  })
}

function isCanceled(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.Canceled
}

/**
 * 操作が履歴からも消えた。
 *
 * 追いかける先が無いので繋ぎ直さない。繰り返しても同じ答えしか
 * 返ってこないため、待って叩き続けるだけになる。
 */
function isNotFound(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.NotFound
}
