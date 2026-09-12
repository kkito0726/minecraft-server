import type { ReactNode } from 'react'

/**
 * 区画の見出しの行。見出し・飾りの英字ラベル・右端の操作を並べる。
 *
 * 英字ラベルは雰囲気のための飾りで、読み上げからは外す。
 * 見出しの名前に混ぜると、見出しで画面を探す人と試験の両方が迷う。
 */
export type PanelHeaderProps = {
  title: string
  /** 飾りの英字ラベル。 */
  tag: string
  /** 見出しの直後に置く小さな印。状態のバッジなど。 */
  badge?: ReactNode | undefined
  /** 右端に寄せる操作。 */
  children?: ReactNode | undefined
}

export function PanelHeader({ title, tag, badge, children }: PanelHeaderProps) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <h2 className="hud-title">{title}</h2>
      {badge}
      <span aria-hidden="true" className="hud-tag hidden sm:inline">
        {tag}
      </span>
      {children && <div className="ml-auto flex flex-wrap gap-2">{children}</div>}
    </div>
  )
}
