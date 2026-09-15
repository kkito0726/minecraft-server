/**
 * OperationService のデモ実装。
 *
 * WatchOperation は実物と同じ約束を守る。fromSeq 以降を再送してから
 * 追従するので、再読み込みしても帯とログがそのまま戻る。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { OperationService } from '../gen/mcadmin/v1/operation_pb'
import { toEvent, toOperation } from './messages'
import { getActiveOperation, getOperation, listOperations, watchOperation } from './operations'
import { notFound } from './rpcErrors'

export const operationImpl: Partial<ServiceImpl<typeof OperationService>> = {
  getOperation: (req) => {
    const operation = getOperation(req.operationId)
    if (!operation) {
      throw notFound(`操作が見つかりません: ${req.operationId}`)
    }
    return { operation: toOperation(operation) }
  },

  getActiveOperation: () => {
    const active = getActiveOperation()
    return active ? { present: true, operation: toOperation(active) } : { present: false }
  },

  listOperations: (req) => ({
    operations: listOperations(req.limit).map(toOperation),
  }),

  watchOperation: async function* (req, ctx) {
    if (!getOperation(req.operationId)) {
      throw notFound(`操作が見つかりません: ${req.operationId}`)
    }
    for await (const event of watchOperation(req.operationId, req.fromSeq, ctx.signal)) {
      yield toEvent(event)
    }
  },
}
