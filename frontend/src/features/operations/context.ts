import { createContext, useContext } from 'react'

import type { OperationStream } from './useOperationStream'

/**
 * 進行中の操作は画面全体で 1 つ。サーバー側がロックで排他しているので、
 * 購読も 1 箇所にまとめる。画面ごとに購読すると、同じストリームに
 * 複数から繋いでログが重複する。
 *
 * コンポーネントと別ファイルにしているのは、React Fast Refresh が
 * 「コンポーネントだけを export するファイル」でしか働かないため。
 */
export const OperationContext = createContext<OperationStream | null>(null)

/** 進行中の操作を参照する。Provider の外で呼ぶと例外になる。 */
export function useOperation(): OperationStream {
  const value = useContext(OperationContext)
  if (!value) {
    throw new Error('OperationProvider の中で使ってください')
  }
  return value
}
