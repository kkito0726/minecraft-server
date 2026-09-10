import type { InputHTMLAttributes } from 'react'

/**
 * チェックボックス。
 *
 * ラベルとの紐付けは molecules が担う。ここは見た目だけを持つ。
 */
export type CheckboxProps = InputHTMLAttributes<HTMLInputElement>

export function Checkbox({ className = '', ...props }: CheckboxProps) {
  return (
    <input
      type="checkbox"
      className={[
        'h-4 w-4 shrink-0 rounded border-gray-300',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      ].join(' ')}
      {...props}
    />
  )
}
