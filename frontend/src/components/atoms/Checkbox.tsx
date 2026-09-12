import type { InputHTMLAttributes } from 'react'

/**
 * チェックボックス。
 *
 * ラベルとの紐付けは molecules が担う。ここは見た目だけを持つ。
 * 部品そのものはブラウザの標準を使い、色だけを accent-color で合わせる。
 * 作り直すと、キーボード操作と読み上げを自前で保証することになる。
 */
export type CheckboxProps = InputHTMLAttributes<HTMLInputElement>

export function Checkbox({ className = '', ...props }: CheckboxProps) {
  return (
    <input
      type="checkbox"
      className={[
        'size-4 shrink-0 cursor-pointer accent-emerald',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-diamond',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      ].join(' ')}
      {...props}
    />
  )
}
