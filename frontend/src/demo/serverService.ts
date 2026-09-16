/**
 * ServerService のデモ実装。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { parseGameSettings } from '../features/settings/settings'
import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import { Difficulty, ServerService } from '../gen/mcadmin/v1/server_pb'
import type { GameSettings } from '../gen/mcadmin/v1/server_pb'
import { toGameSettings, toOperation, toStatus } from './messages'
import { isBusy, startOperation } from './operations'
import { busyGuard, failedPrecondition, invalidArgument } from './rpcErrors'
import { getState, updateState } from './state'
import type { DemoGameSettings } from './state'
import { APPLY_SETTINGS_STEPS, RESTART_STEPS, START_STEPS, STOP_STEPS } from './steps'

const DIFFICULTY_NAMES: Partial<Record<Difficulty, DemoGameSettings['difficulty']>> = {
  [Difficulty.PEACEFUL]: 'peaceful',
  [Difficulty.EASY]: 'easy',
  [Difficulty.NORMAL]: 'normal',
  [Difficulty.HARD]: 'hard',
}

/**
 * 画面と同じ規則で検証する。
 *
 * 画面の検証をそのまま使うので、デモと実物で弾かれる値がずれない。
 * 実物のバックエンドとの一致は、画面側の試験が Go のソースを読んで確かめている。
 */
function validated(input: GameSettings | undefined): DemoGameSettings {
  const result = parseGameSettings({
    difficulty: input?.difficulty ?? Difficulty.UNSPECIFIED,
    motd: input?.motd ?? '',
    maxPlayers: String(input?.maxPlayers ?? ''),
    viewDistance: String(input?.viewDistance ?? ''),
    simulationDistance: String(input?.simulationDistance ?? ''),
  })
  if (!result.ok) {
    throw invalidArgument(result.message)
  }
  const difficulty = DIFFICULTY_NAMES[result.value.difficulty]
  if (!difficulty) {
    throw invalidArgument('難易度が指定されていません')
  }
  return { ...result.value, difficulty }
}

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

  getGameSettings: () => ({
    settings: toGameSettings(getState().gameSettings),
    warnings: [],
  }),

  updateGameSettings: (req) => {
    const next = validated(req.settings)
    const settings = toGameSettings(next)
    // 実物と同じく、他の操作の最中は書かない。相手の書き込みを競合させる。
    if (isBusy()) {
      throw failedPrecondition('他の操作が実行中です')
    }

    // 保存だけ、あるいは停止中。書くだけで、動いているサーバーの人数は変えない。
    if (!req.applyNow || !getState().running) {
      updateState((s) => ({ ...s, gameSettings: next }))
      return { settings }
    }

    return busyGuard(() => {
      const operation = startOperation({
        kind: OperationKind.SERVER_APPLY_SETTINGS,
        steps: APPLY_SETTINGS_STEPS,
        commit: () =>
          updateState((s) => ({
            ...s,
            maxPlayers: next.maxPlayers,
            onlinePlayers: 0,
            running: true,
            healthy: true,
            startedAt: new Date(),
          })),
      })
      updateState((s) => ({ ...s, gameSettings: next }))
      return { settings, operation: toOperation(operation) }
    })
  },
}
