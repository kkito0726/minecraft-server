import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'

import type { GetGameSettingsResponse } from '../../gen/mcadmin/v1/server_pb'
import { queryKeys } from '../../lib/queryKeys'
import { useOperation } from '../operations'
import { serverClient } from '../server/client'
import type { GameSettingsValues } from './settings'

/** .env のゲーム設定を読む。 */
export function useGameSettings(): UseQueryResult<GetGameSettingsResponse> {
  return useQuery({
    queryKey: queryKeys.gameSettings,
    queryFn: () => serverClient.getGameSettings({}),
  })
}

export type UpdateGameSettingsInput = {
  settings: GameSettingsValues
  /** 真なら、稼働中のサーバーを作り直して今すぐ反映する。 */
  applyNow: boolean
}

/**
 * ゲーム設定を保存する。
 *
 * 今すぐ反映を選んで稼働中なら、作り直しの操作が返る。その場で購読へ
 * 差し込む。保存だけ（あるいは停止中）なら操作は返らず、.env を読み直す。
 */
export function useUpdateGameSettings() {
  const { begin } = useOperation()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ settings, applyNow }: UpdateGameSettingsInput) =>
      serverClient.updateGameSettings({ settings, applyNow }),
    onSuccess: (res) => {
      if (res.operation) {
        begin(res.operation)
      }
      return queryClient.invalidateQueries({ queryKey: queryKeys.gameSettings })
    },
  })
}
