/**
 * ServerService のデモ実装。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import { ServerService } from '../gen/mcadmin/v1/server_pb'
import { toOperation, toStatus } from './messages'
import { startOperation } from './operations'
import { busyGuard } from './rpcErrors'
import { getState, updateState } from './state'
import { RESTART_STEPS, START_STEPS, STOP_STEPS } from './steps'

/** 画面が読んでよい設定。実物の publicConfigKeys に合わせる。 */
function configValues(): Record<string, string> {
  const state = getState()
  return {
    MC_VERSION: state.configuredVersion,
    MC_TYPE: 'PAPER',
    MC_LEVEL: state.activeLevel,
    MC_MOTD: 'デモ用のサーバーです',
    MC_DIFFICULTY: 'normal',
    MC_MAX_PLAYERS: String(state.maxPlayers),
    MC_MEMORY: '2G',
    TZ: 'Asia/Tokyo',
  }
}

export const serverImpl: Partial<ServiceImpl<typeof ServerService>> = {
  getStatus: () => toStatus(getState()),

  getConfig: () => ({ values: configValues() }),

  startServer: () =>
    busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.SERVER_START,
          steps: START_STEPS,
          commit: () =>
            updateState((s) => ({
              ...s,
              running: true,
              healthy: true,
              startedAt: new Date(),
              onlinePlayers: 0,
            })),
        }),
      ),
    })),

  stopServer: () =>
    busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.SERVER_STOP,
          steps: STOP_STEPS,
          commit: () =>
            updateState((s) => ({
              ...s,
              running: false,
              healthy: false,
              startedAt: null,
              onlinePlayers: 0,
            })),
        }),
      ),
    })),

  restartServer: () =>
    busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.SERVER_RESTART,
          steps: RESTART_STEPS,
          commit: () =>
            updateState((s) => ({
              ...s,
              running: true,
              healthy: true,
              startedAt: new Date(),
            })),
        }),
      ),
    })),
}
