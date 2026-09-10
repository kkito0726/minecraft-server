import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { OperationProvider } from '../features/operations'
import type { OperationSource } from '../features/operations'

/**
 * 画面の部品を試験するための入れ物。
 *
 * OperationProvider は完了時に問い合わせを無効化するため
 * QueryClientProvider の中でしか動かない。本番の入れ子と同じ順序にする。
 */
export function withProviders(children: ReactNode, source?: OperationSource) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return (
    <QueryClientProvider client={client}>
      <OperationProvider source={source}>{children}</OperationProvider>
    </QueryClientProvider>
  )
}
