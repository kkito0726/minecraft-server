import type { ReactNode } from 'react'

/**
 * ラベル。状態や分類を短く示す。
 *
 * tone は見た目の強さであって意味ではない。
 * 「このバージョンは古い」という判断は molecules 以上で行う。
 * 先頭の点は疑似要素で描く。文字として足すと、読み上げと
 * 文言での検索の両方に紛れ込む。
 */
export type BadgeTone = 'neutral' | 'ok' | 'warn' | 'danger'

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'badge-neutral',
  ok: 'badge-ok',
  warn: 'badge-warn',
  danger: 'badge-danger',
}

export type BadgeProps = {
  tone?: BadgeTone
  children: ReactNode
}

export function Badge({ tone = 'neutral', children }: BadgeProps) {
  return <span className={`badge ${toneClasses[tone]}`}>{children}</span>
}
