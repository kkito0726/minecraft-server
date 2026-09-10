import type { ReactNode } from 'react'

/**
 * 画面全体の骨組み。
 *
 * templates は配置だけを決め、実データを持たない。
 * ナビゲーションの中身も進捗バナーも、呼び出し側が差し込む。
 */
export type AppShellProps = {
  /** ヘッダー左側。アプリ名など。 */
  brand: ReactNode
  /** ヘッダー右側のナビゲーション。 */
  nav: ReactNode
  /**
   * ヘッダー直下に貼り付く帯。
   * 進行中の操作や中断の警告を出す場所。
   */
  banner?: ReactNode | undefined
  children: ReactNode
}

export function AppShell({ brand, nav, banner, children }: AppShellProps) {
  return (
    <div className="flex min-h-full flex-col bg-gray-50">
      <header className="sticky top-0 z-10 border-b border-gray-200 bg-white">
        <div className="mx-auto flex max-w-5xl items-center gap-4 px-4 py-3">
          <div className="font-semibold text-gray-900">{brand}</div>
          <nav className="flex gap-1">{nav}</nav>
        </div>
        {banner}
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6">{children}</main>
    </div>
  )
}
