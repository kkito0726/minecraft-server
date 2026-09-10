import { QueryClient } from '@tanstack/react-query'

import { isUnauthenticated } from './errors'

/**
 * 問い合わせと変更の既定を決める。
 *
 * 認可の失敗は何度やっても同じ結果になるので再試行しない。
 * 変更系（起動・復元・削除）は副作用があるため、そもそも自動で
 * やり直さない。二重にバックアップが走ったり、復元が二回走ったりする。
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => !isUnauthenticated(error) && failureCount < 2,
        // サーバーの状態は操作の完了時に明示的に無効化する。
        // 画面を切り替えるたびに取り直す必要はない。
        refetchOnWindowFocus: false,
        staleTime: 5_000,
      },
      mutations: {
        retry: false,
      },
    },
  })
}
