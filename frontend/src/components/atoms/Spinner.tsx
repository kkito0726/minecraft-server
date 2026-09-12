/**
 * 処理中であることを示す。
 *
 * 何の処理かは知らない。呼び出し側が label で伝える。
 */
export type SpinnerProps = {
  /** 支援技術に読み上げさせる説明。 */
  label?: string | undefined
  /**
   * 飾りとして扱い、支援技術から隠す。
   *
   * すでに状況を読み上げている領域の中に置くときに使う。
   * ライブリージョンが入れ子になると、同じ進捗が二重に読み上げられる。
   */
  decorative?: boolean | undefined
}

export function Spinner({ label = '処理中', decorative = false }: SpinnerProps) {
  if (decorative) {
    return <span aria-hidden="true" className="block-spinner" />
  }
  return <span role="status" aria-label={label} className="block-spinner" />
}
