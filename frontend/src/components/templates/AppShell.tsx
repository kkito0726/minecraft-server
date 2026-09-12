import type { ReactNode } from 'react'

/**
 * 画面全体の骨組み。
 *
 * templates は配置だけを決め、実データを持たない。
 * ナビゲーションの中身も進捗バナーも、呼び出し側が差し込む。
 *
 * 広い画面では左に縦のナビゲーションを固定し、狭い画面では
 * 上に横並びで出す。切り替えは CSS だけで行い、ナビゲーションの
 * リンクは画面幅にかかわらず常に描画する。
 */
export type AppShellProps = {
  /** 左上の名札。アプリ名など。 */
  brand: ReactNode
  /** ナビゲーションのリンク。 */
  nav: ReactNode
  /** 上端に常に出す概況。どの画面にいても見える。 */
  status?: ReactNode | undefined
  /**
   * 概況の直下に貼り付く帯。
   * 進行中の操作や中断の警告を出す場所。
   */
  banner?: ReactNode | undefined
  /** 広い画面でだけ、ナビゲーションの下に出す飾り。 */
  footer?: ReactNode | undefined
  children: ReactNode
}

export function AppShell({ brand, nav, status, banner, footer, children }: AppShellProps) {
  return (
    <div className="flex min-h-full flex-col lg:grid lg:grid-cols-[15.5rem_minmax(0,1fr)]">
      <aside className="flex flex-col border-b border-line bg-panel/80 backdrop-blur lg:sticky lg:top-0 lg:h-dvh lg:self-start lg:border-r lg:border-b-0">
        <div className="px-4 pt-4 pb-3 lg:px-5 lg:pt-7 lg:pb-8">{brand}</div>
        <nav className="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:overflow-visible lg:pb-0">
          {nav}
        </nav>
        {footer && <div className="mt-auto hidden px-5 py-6 lg:block">{footer}</div>}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10">
          {status}
          {banner}
        </header>
        <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 lg:px-8 lg:py-8">{children}</main>
      </div>
    </div>
  )
}
