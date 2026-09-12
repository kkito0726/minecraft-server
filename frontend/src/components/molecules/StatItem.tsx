import type { ReactNode } from 'react'

/**
 * 「項目名 + 値」の 1 組。状態カードを組み立てる最小単位。
 *
 * 計器の読み取り窓として見せる。右上の通し番号は .stat-grid の
 * CSS カウンターが振るので、並べる側は順序だけを決めればよい。
 */
export type StatItemProps = {
  label: string
  children: ReactNode
}

export function StatItem({ label, children }: StatItemProps) {
  return (
    <div className="stat-tile flex flex-col gap-1.5">
      <dt className="text-[11px] font-medium tracking-wide text-faint sm:pr-5">{label}</dt>
      <dd className="font-pixel text-lg leading-snug break-words text-fg">{children}</dd>
    </div>
  )
}
