import { useMemo } from 'react'
import type { ReactNode } from 'react'

import { OperationContext } from './context'
import { useOperationStream } from './useOperationStream'
import { createOperationSource } from './watch'
import type { OperationSource } from './watch'

export type OperationProviderProps = {
  /** 購読元。省略すると実際のサーバーに繋ぐ。テストで差し替える。 */
  source?: OperationSource | undefined
  children: ReactNode
}

export function OperationProvider({ source, children }: OperationProviderProps) {
  // 参照が変わるたびに購読が張り直される。1 回だけ作る。
  const resolved = useMemo(() => source ?? createOperationSource(), [source])
  const stream = useOperationStream(resolved)

  return <OperationContext.Provider value={stream}>{children}</OperationContext.Provider>
}
