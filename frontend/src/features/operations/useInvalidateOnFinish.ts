import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'

import { invalidatedOnOperationFinish } from '../../lib/queryKeys'
import { isTerminal } from './operationReducer'
import type { OperationStream } from './useOperationStream'

/**
 * 操作が終わったら、画面が持っている情報を取り直させる。
 *
 * 1 箇所で行うのが要点。画面ごとに書くと、その画面を開いていない
 * ときに無効化されず、戻ってきたときに古い状態が出る。
 *
 * 操作 1 回につき 1 度だけ発火させる。終端に達した後も再描画は
 * 起きるため、印を付けておかないと取得を繰り返す。
 */
export function useInvalidateOnFinish(stream: OperationStream): void {
  const queryClient = useQueryClient()
  const invalidated = useRef<string | null>(null)

  const operation = stream.operation
  useEffect(() => {
    if (!operation || !isTerminal(operation) || invalidated.current === operation.id) {
      return
    }
    invalidated.current = operation.id

    for (const queryKey of invalidatedOnOperationFinish) {
      void queryClient.invalidateQueries({ queryKey })
    }
  }, [operation, queryClient])
}
