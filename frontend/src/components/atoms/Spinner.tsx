/**
 * 処理中であることを示す。
 *
 * 何の処理かは知らない。呼び出し側が label で伝える。
 */
export type SpinnerProps = {
  /** 支援技術に読み上げさせる説明。 */
  label?: string | undefined
}

export function Spinner({ label = '処理中' }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-label={label}
      className="inline-block size-4 animate-spin rounded-full border-2 border-gray-300 border-t-gray-700"
    />
  )
}
