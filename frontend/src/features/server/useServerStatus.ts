import { useQuery } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'

import type { GetStatusResponse } from '../../gen/mcadmin/v1/server_pb'
import { queryKeys } from '../../lib/queryKeys'
import { serverClient } from './client'

/**
 * サーバーの状態を取得する。
 *
 * 定期的な再取得はしない。状態が変わるのは操作の前後だけで、
 * 操作の完了時に無効化される（OperationProvider が行う）。
 * Pi の負荷を無駄に増やさない。
 */
export function useServerStatus(): UseQueryResult<GetStatusResponse> {
  return useQuery({
    queryKey: queryKeys.status,
    queryFn: () => serverClient.getStatus({}),
  })
}
