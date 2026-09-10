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
        'h-4 w-4 shrink-0 border-gray-300',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      ].join(' ')}
      {...props}
    />
  )
}
