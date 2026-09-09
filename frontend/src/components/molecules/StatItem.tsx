import type { ReactNode } from 'react'

/**
 * 「項目名 + 値」の 1 組。状態カードを組み立てる最小単位。
 */
export type StatItemProps = {
  label: string
  children: ReactNode
}

export function StatItem({ label, children }: StatItemProps) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs text-gray-500">{label}</dt>
      <dd className="text-sm font-medium text-gray-900">{children}</dd>
    </div>
  )
}
