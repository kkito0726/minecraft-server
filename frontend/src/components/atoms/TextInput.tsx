import type { InputHTMLAttributes } from 'react'

/**
 * 一行のテキスト入力。
 *
 * ラベルやエラー表示は持たない。それらは molecules の FormField が担う。
 */
export type TextInputProps = InputHTMLAttributes<HTMLInputElement> & {
  invalid?: boolean
}

export function TextInput({ invalid = false, className = '', ...props }: TextInputProps) {
  return (
    <input
      type="text"
      aria-invalid={invalid || undefined}
      className={[
        'w-full rounded border px-2 py-1.5 text-sm',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1',
        'disabled:cursor-not-allowed disabled:bg-gray-50',
        invalid ? 'border-danger-500' : 'border-gray-300',
        className,
      ].join(' ')}
      {...props}
    />
  )
}
