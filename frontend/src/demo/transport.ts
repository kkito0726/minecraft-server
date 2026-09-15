/**
 * デモ用のトランスポート。
 *
 * createRouterTransport は、サービスの実装をブラウザの中でそのまま動かす。
 * 手書きのスタブと違い、生成された proto の型を通り、server-streaming も
 * 本物と同じ経路を通る。画面のコードは実物との違いを知らないままでよい。
 */
import { createRouterTransport } from '@connectrpc/connect'
import type { Transport } from '@connectrpc/connect'

import { BackupService } from '../gen/mcadmin/v1/backup_pb'
import { OperationService } from '../gen/mcadmin/v1/operation_pb'
import { ServerService } from '../gen/mcadmin/v1/server_pb'
import { WorldService } from '../gen/mcadmin/v1/world_pb'
import { backupImpl } from './backupService'
import { operationImpl } from './operationService'
import { serverImpl } from './serverService'
import { worldImpl } from './worldService'

export function createDemoTransport(): Transport {
  return createRouterTransport((router) => {
    router.service(ServerService, serverImpl)
    router.service(WorldService, worldImpl)
    router.service(BackupService, backupImpl)
    router.service(OperationService, operationImpl)
  })
}
