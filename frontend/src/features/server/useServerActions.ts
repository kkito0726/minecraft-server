import { useMutation } from '@tanstack/react-query'

import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import { useOperation } from '../operations'
import { serverClient } from './client'

export type ServerAction = 'start' | 'stop' | 'restart'

const CALLS: Record<ServerAction, () => Promise<{ operation?: Operation | undefined }>> = {
  start: () => serverClient.startServer({}),
  stop: () => serverClient.stopServer({}),
  restart: () => serverClient.restartServer({}),
}

export const ACTION_LABELS: Record<ServerAction, string> = {
  start: '起動',
  stop: '停止',
  restart: '再起動',
}

/**
 * サーバーの起動・停止・再起動。
 *
 * 応答に含まれる Operation をその場で購読へ差し込む。定期的な探索を
 * 待つと、押してから帯が出るまで数秒何も起きない時間ができる。
 */
export function useServerActions() {
  const { begin } = useOperation()

  return useMutation({
    mutationFn: (action: ServerAction) => CALLS[action](),
    onSuccess: (res) => {
      if (res.operation) {
        begin(res.operation)
      }
    },
  })
}
