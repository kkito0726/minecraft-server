import { useMutation, useQuery } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'

import type { Difficulty, GameMode } from '../../gen/mcadmin/v1/server_pb'
import type { ListVersionsResponse, ListWorldsResponse } from '../../gen/mcadmin/v1/world_pb'
import { queryKeys } from '../../lib/queryKeys'
import { useOperation } from '../operations'
import { worldClient } from './client'

/**
 * ワールドを作れる版の一覧。作成のダイアログを開いたときだけ読む。
 *
 * 一覧はバックエンドが Paper から取る。取れなくても失敗にはならず、
 * catalogAvailable が偽で現在の版だけが返る。
 */
export function useVersions(): UseQueryResult<ListVersionsResponse> {
  return useQuery({
    queryKey: queryKeys.versions,
    queryFn: () => worldClient.listVersions({}),
    // Paper の一覧は数週間に一度しか変わらない。開くたびに問い合わせない。
    staleTime: 10 * 60 * 1000,
  })
}

export function useWorlds(): UseQueryResult<ListWorldsResponse> {
  return useQuery({
    queryKey: queryKeys.worlds,
    queryFn: () => worldClient.listWorlds({}),
  })
}

/** ワールドに対する変更。どれも操作を返すので、その場で購読へ差し込む。 */
export type WorldCommand =
  | { kind: 'switch'; name: string }
  | {
      kind: 'create'
      name: string
      seed: string
      // 生成の前に .env へ書く。効くのが生成の瞬間だけなので作成時に受け取る。
      mode: GameMode
      difficulty: Difficulty
      hardcore: boolean
      version: string
    }
  | { kind: 'clone'; source: string; destination: string }
  | { kind: 'rename'; from: string; to: string }
  | { kind: 'delete'; name: string; confirmName: string }

async function run(command: WorldCommand) {
  switch (command.kind) {
    case 'switch':
      return worldClient.switchWorld({ name: command.name })
    case 'create':
      return worldClient.createWorld({
        name: command.name,
        seed: command.seed,
        mode: command.mode,
        difficulty: command.difficulty,
        hardcore: command.hardcore,
        version: command.version,
      })
    case 'clone':
      return worldClient.cloneWorld({
        source: command.source,
        destination: command.destination,
      })
    case 'rename':
      return worldClient.renameWorld({ from: command.from, to: command.to })
    case 'delete':
      // 退避を既定にする。復元と同じく、まず mv してから人が消す。
      return worldClient.deleteWorld({
        name: command.name,
        confirmName: command.confirmName,
        quarantineInstead: true,
      })
  }
}

export function useWorldCommand() {
  const { begin } = useOperation()

  return useMutation({
    mutationFn: run,
    onSuccess: (res) => {
      if (res.operation) {
        begin(res.operation)
      }
    },
  })
}

/**
 * 退避されたディレクトリを完全に削除する。
 *
 * これだけは操作にならない。時間がかからず、ワールドの実体も動かさない。
 */
export function usePurgeQuarantine() {
  return useMutation({
    mutationFn: (name: string) => worldClient.purgeQuarantine({ name }),
  })
}
