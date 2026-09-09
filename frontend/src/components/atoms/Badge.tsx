import type { ReactNode } from 'react'

/**
 * ラベル。状態や分類を短く示す。
 *
 * tone は見た目の強さであって意味ではない。
 * 「このバージョンは古い」という判断は molecules 以上で行う。
 */
export type BadgeTone = 'neutral' | 'ok' | 'warn' | 'danger'

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'bg-gray-100 text-gray-700',
  ok: 'bg-ok-50 text-ok-700',
  warn: 'bg-warn-50 text-warn-700',
  danger: 'bg-danger-50 text-danger-700',
}

export type BadgeProps = {
  tone?: BadgeTone
  children: ReactNode
}

export function Badge({ tone = 'neutral', children }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${toneClasses[tone]}`}
    >
      {children}
    </span>
  )
}
