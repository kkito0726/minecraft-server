import { createClient } from '@connectrpc/connect'

import { OperationService } from '../../gen/mcadmin/v1/operation_pb'
import type { Operation, WatchOperationResponse } from '../../gen/mcadmin/v1/operation_pb'
import { transport } from '../../lib/transport'

/**
 * 操作の購読に必要な最小の口。
 *
 * インターフェースにしておくのは、フックのテストで実物の通信を
 * 使わずに「切断されたら再接続するか」を確かめるため。
 */
export type OperationSource = {
  /** 進行中の操作を返す。無ければ null。 */
  active: () => Promise<Operation | null>
  /** fromSeq 以降のイベントを配信する。 */
  watch: (
    operationId: string,
    fromSeq: bigint,
    signal: AbortSignal,
  ) => AsyncIterable<WatchOperationResponse>
}

/** 実際のサーバーに繋ぐ実装。 */
export function createOperationSource(): OperationSource {
  const client = createClient(OperationService, transport)

  return {
    active: async () => {
      const res = await client.getActiveOperation({})
      return res.present ? (res.operation ?? null) : null
    },
    watch: (operationId, fromSeq, signal) =>
      client.watchOperation({ operationId, fromSeq }, { signal }),
  }
}
