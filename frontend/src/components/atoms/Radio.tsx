import type { InputHTMLAttributes } from 'react'

/**
 * ラジオボタン。
 *
 * 排他の選択肢そのものは molecules の ChoiceGroup が組み立てる。
 */
export type RadioProps = InputHTMLAttributes<HTMLInputElement>

export function Radio({ className = '', ...props }: RadioProps) {
  return (
    <input
      type="radio"
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
