import { useQuery } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'

import type { GetMetricsResponse } from '../../gen/mcadmin/v1/system_pb'
import { queryKeys } from '../../lib/queryKeys'
import { systemClient } from './client'

/** 画面を開いている間の取得間隔。CPU 使用率の測定区間もこれになる。 */
export const METRICS_INTERVAL_MS = 5_000

/**
 * ホストの資源の使用状況を取得する。
 *
 * この画面だけは定期的に取り直す。他の画面が取り直さないのは状態が
 * 操作の前後でしか変わらないからで、ここは常に動いている値を見に来る
 * 場所なので前提が違う。読むのは /proc と statfs だけでプロセスも
 * 生やさないため、5 秒間隔でも Pi の負荷にはならない。
 *
 * この hook を使う画面を離れれば購読ごと止まる。
 */
export function useSystemMetrics(): UseQueryResult<GetMetricsResponse> {
  return useQuery({
    queryKey: queryKeys.systemMetrics,
    queryFn: () => systemClient.getMetrics({}),
    refetchInterval: METRICS_INTERVAL_MS,
    // 取り直しの合間も前の値を出し続ける。5 秒ごとに数字が消えて
    // 現れ直すと、読んでいる最中に読めなくなる。
    placeholderData: (previous) => previous,
  })
}
