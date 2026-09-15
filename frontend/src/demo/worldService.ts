/**
 * WorldService のデモ実装。
 *
 * 取り消せない操作（削除）は、実物と同じく名前の完全一致を要求する。
 * デモだからと素通りさせると、関門があること自体が伝わらない。
 */
import type { ServiceImpl } from '@connectrpc/connect'

import { OperationKind } from '../gen/mcadmin/v1/common_pb'
import { WorldService } from '../gen/mcadmin/v1/world_pb'
import { toOperation, toQuarantine, toWorld } from './messages'
import { startOperation } from './operations'
import { busyGuard, invalidArgument, notFound } from './rpcErrors'
import { getState, updateState } from './state'
import type { DemoWorld } from './state'
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

export const worldImpl: Partial<ServiceImpl<typeof WorldService>> = {
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
          commit: () => updateState((s) => ({ ...s, activeLevel: req.name })),
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
              worlds: [
                ...s.worlds,
                {
                  name: req.name,
                  sizeBytes: 12_800_000n,
                  lastPlayed: new Date(),
                  version: {
                    readable: true,
                    name: s.configuredVersion,
                    dataVersion: 4903,
                    levelName: req.name,
                  },
                  hasSessionLock: true,
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
