/**
 * WorldService のデモ実装。
 *
 * 取り消せない操作（削除）は、実物と同じく名前の完全一致を要求する。
 * デモだからと素通りさせると、関門があること自体が伝わらない。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import { Difficulty, GameMode } from '../gen/mcadmin/v1/server_pb'
import { WorldService } from '../gen/mcadmin/v1/world_pb'
import { toOperation, toQuarantine, toWorld } from './messages'
import { startOperation } from './operations'
import { busyGuard, invalidArgument, notFound } from './rpcErrors'
import { getState, updateState } from './state'
import type { DemoGameSettings, DemoWorld } from './state'
import { CLONE_STEPS, CREATE_WORLD_STEPS, DELETE_STEPS, RENAME_STEPS, SWITCH_STEPS } from './steps'

function stamp(): string {
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
}

function requireWorld(name: string): DemoWorld {
  const world = getState().worlds.find((w) => w.name === name)
  if (!world) {
    throw notFound(`ワールドが見つかりません: ${name}`)
  }
  return world
}

/** 未指定なら触らない。実物の CreateWorld と同じ約束。 */
const MODE_NAMES: Partial<Record<GameMode, DemoGameSettings['mode']>> = {
  [GameMode.SURVIVAL]: 'survival',
  [GameMode.CREATIVE]: 'creative',
  [GameMode.ADVENTURE]: 'adventure',
  [GameMode.SPECTATOR]: 'spectator',
}

const DIFFICULTY_NAMES: Partial<Record<Difficulty, DemoGameSettings['difficulty']>> = {
  [Difficulty.PEACEFUL]: 'peaceful',
  [Difficulty.EASY]: 'easy',
  [Difficulty.NORMAL]: 'normal',
  [Difficulty.HARD]: 'hard',
}

/**
 * デモで選べる版。実物は Paper の API から取るが、デモは外へ出ないので
 * 2026-09 時点の一覧の一部を持っておく。
 */
const DEMO_VERSIONS = ['26.3', '26.2', '26.1.2', '26.1.1', '1.21.11', '1.21.4', '1.20.6', '1.20.1']

export const worldImpl: Partial<ServiceImpl<typeof WorldService>> = {
  listVersions: () => ({
    versions: DEMO_VERSIONS,
    current: getState().configuredVersion,
    catalogAvailable: true,
    unavailableReason: '',
  }),

  listWorlds: () => {
    const state = getState()
    return {
      worlds: state.worlds.map((w) => toWorld(state, w)),
      quarantines: state.quarantines.map(toQuarantine),
      activeLevel: state.activeLevel,
    }
  },

  switchWorld: (req) => {
    requireWorld(req.name)
    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.WORLD_SWITCH,
          steps: SWITCH_STEPS,
          attributes: { world_name: req.name },
          // 実物と同じく、切り替え先のワールドの印に .env を合わせる。
          // ハードコアに入るときは難易度もハードにする。
          commit: () =>
            updateState((s) => {
              const target = s.worlds.find((w) => w.name === req.name)
              const hardcore = target?.hardcore ?? false
              return {
                ...s,
                activeLevel: req.name,
                // 実物と同じく、切り替え先のワールドの版にサーバーを合わせる。
                configuredVersion: target?.version.readable
                  ? target.version.name
                  : s.configuredVersion,
                gameSettings: {
                  ...s.gameSettings,
                  hardcore,
                  ...(hardcore ? { difficulty: 'hard' as const } : {}),
                },
              }
            }),
        }),
      ),
    }))
  },

  createWorld: (req) => {
    if (getState().worlds.some((w) => w.name === req.name)) {
      throw invalidArgument(`同じ名前のワールドがあります: ${req.name}`)
    }

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.WORLD_CREATE,
          steps: CREATE_WORLD_STEPS,
          attributes: { world_name: req.name },
          commit: () =>
            updateState((s) => ({
              ...s,
              activeLevel: req.name,
              configuredVersion: req.version || s.configuredVersion,
              // 実物と同じく、生成の前に .env へ書く値をここで反映する。
              // ワールドごとには持たない。切り替えても戻らない。
              gameSettings: {
                ...s.gameSettings,
                ...(MODE_NAMES[req.mode] ? { mode: MODE_NAMES[req.mode] } : {}),
                ...(DIFFICULTY_NAMES[req.difficulty]
                  ? { difficulty: DIFFICULTY_NAMES[req.difficulty] }
                  : {}),
                hardcore: req.hardcore,
              },
              worlds: [
                ...s.worlds,
                {
                  name: req.name,
                  sizeBytes: 12_800_000n,
                  lastPlayed: new Date(),
                  version: {
                    readable: true,
                    name: req.version || s.configuredVersion,
                    dataVersion: 4903,
                    levelName: req.name,
                  },
                  hasSessionLock: true,
                  hardcore: req.hardcore,
                },
              ],
            })),
        }),
      ),
    }))
  },

  cloneWorld: (req) => {
    const source = requireWorld(req.source)
    if (getState().worlds.some((w) => w.name === req.destination)) {
      throw invalidArgument(`同じ名前のワールドがあります: ${req.destination}`)
    }

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.WORLD_CLONE,
          steps: CLONE_STEPS,
          totalBytes: source.sizeBytes,
          attributes: { world_name: req.destination },
          commit: () =>
            updateState((s) => ({
              ...s,
              worlds: [
                ...s.worlds,
                {
                  ...source,
                  name: req.destination,
                  lastPlayed: new Date(),
                  hasSessionLock: false,
                  version: { ...source.version, levelName: req.destination },
                },
              ],
            })),
        }),
      ),
    }))
  },

  renameWorld: (req) => {
    requireWorld(req.from)
    if (getState().worlds.some((w) => w.name === req.to)) {
      throw invalidArgument(`同じ名前のワールドがあります: ${req.to}`)
    }

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.WORLD_RENAME,
          steps: RENAME_STEPS,
          attributes: { world_name: req.to },
          commit: () =>
            updateState((s) => ({
              ...s,
              activeLevel: s.activeLevel === req.from ? req.to : s.activeLevel,
              worlds: s.worlds.map((w) =>
                w.name === req.from
                  ? { ...w, name: req.to, version: { ...w.version, levelName: req.to } }
                  : w,
              ),
            })),
        }),
      ),
    }))
  },

  deleteWorld: (req) => {
    const world = requireWorld(req.name)
    // 実物と同じ関門。押し間違いがそのままデータの喪失になるため。
    if (req.confirmName !== req.name) {
      throw invalidArgument('確認のために入力した名前が一致していません')
    }
    if (getState().activeLevel === req.name) {
      throw invalidArgument('稼働中のワールドは削除できません。先に切り替えてください')
    }

    return busyGuard(() => ({
      operation: toOperation(
        startOperation({
          kind: OperationKind.WORLD_DELETE,
          steps: DELETE_STEPS,
          attributes: { world_name: req.name },
          commit: () =>
            updateState((s) => ({
              ...s,
              worlds: s.worlds.filter((w) => w.name !== req.name),
              quarantines: [
                {
                  name: `${req.name}.deleted-${stamp()}`,
                  originalLevel: req.name,
                  quarantinedAt: new Date(),
                  sizeBytes: world.sizeBytes,
                  fromRestore: false,
                },
                ...s.quarantines,
              ],
            })),
        }),
      ),
    }))
  },

  /** 退避の完全削除。実物と同じく、これだけは操作にしない。 */
  purgeQuarantine: (req) => {
    const target = getState().quarantines.find((q) => q.name === req.name)
    if (!target) {
      throw notFound(`退避が見つかりません: ${req.name}`)
    }

    updateState((s) => ({
      ...s,
      quarantines: s.quarantines.filter((q) => q.name !== req.name),
    }))
    return { freedBytes: target.sizeBytes }
  },
}
