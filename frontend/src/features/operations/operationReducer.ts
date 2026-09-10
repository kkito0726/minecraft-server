import { OperationState } from '../../gen/mcadmin/v1/common_pb'
import type { LogLevel } from '../../gen/mcadmin/v1/common_pb'
import type { Operation, WatchOperationResponse } from '../../gen/mcadmin/v1/operation_pb'

/** 画面に出す 1 行のログ。 */
export type LogLine = {
  seq: bigint
  level: LogLevel
  message: string
  at: Date | undefined
}

/**
 * 進捗の状態。
 *
 * 真実はサーバーにあり、ここはその写しでしかない。画面は
 * 「今どの操作を見ているか」だけを持ち、長い処理そのものは持たない。
 */
export type OperationStreamState = {
  operation: Operation | null
  log: LogLine[]
  /** 受け取った最大の seq。再接続の位置指定に使う。 */
  lastSeq: bigint
}

export const initialState: OperationStreamState = {
  operation: null,
  log: [],
  lastSeq: 0n,
}

export type OperationAction =
  /** ストリームから 1 イベント受け取った。 */
  | { type: 'event'; event: WatchOperationResponse }
  /** 変更系の応答から新しい操作を差し込む。 */
  | { type: 'begin'; operation: Operation }
  /** 終わった操作の表示を閉じる。 */
  | { type: 'dismiss' }

/**
 * 進捗の状態を進める純関数。
 *
 * 既存のオブジェクトは変更せず、常に新しいものを返す。
 */
export function operationReducer(
  state: OperationStreamState,
  action: OperationAction,
): OperationStreamState {
  switch (action.type) {
    case 'event':
      return applyEvent(state, action.event)
    case 'begin':
      // 同じ操作を二度差し込まない。定期的な探索と変更系の応答は
      // どちらも begin を呼ぶため、サーバーが操作を作ってから
      // 応答が返るまでの間に探索が当たると二重になる。
      //
      // 状態ごと据え置くのが要点。応答に含まれる snapshot は
      // PENDING なので、取り込むと RUNNING の表示が巻き戻る。
      if (state.operation?.id === action.operation.id) {
        return state
      }
      // 別の操作の始まり。進行中の操作のイベントではないので
      // ログと seq を初期化する。
      return { operation: action.operation, log: [], lastSeq: 0n }
    case 'dismiss':
      return initialState
  }
}

/**
 * 同じイベントは 3 つの経路で届く。起動時の fromSeq=0 の再送、
 * 再接続時の fromSeq=lastSeq+1、変更系の応答からの差し込み。
 * 受信済みを落とさないと、再接続のたびにログが二重・三重になる。
 *
 * この番人がいるおかげで、再接続側は多めに取り直しても構わない。
 */
function applyEvent(
  state: OperationStreamState,
  event: WatchOperationResponse,
): OperationStreamState {
  if (event.seq <= state.lastSeq) {
    return state
  }

  const line: LogLine = {
    seq: event.seq,
    level: event.level,
    message: event.message,
    at: event.at ? new Date(Number(event.at.seconds) * 1000) : undefined,
  }

  return {
    // snapshot は毎回まるごと置き換える。1 件取りこぼしても
    // 次のイベントで表示が正しい状態に戻る。
    operation: event.snapshot ?? state.operation,
    log: [...state.log, line],
    lastSeq: event.seq,
  }
}

/** 終端に達していれば真。終端では次の操作を始められる。 */
export function isTerminal(operation: Operation | null): boolean {
  if (!operation) {
    return false
  }
  return (
    operation.state === OperationState.SUCCEEDED || operation.state === OperationState.FAILED
  )
}
