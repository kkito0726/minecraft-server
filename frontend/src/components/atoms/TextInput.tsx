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
        'w-full border px-2.5 py-2 text-sm text-fg placeholder:text-faint',
        'shadow-[inset_0_2px_0_0_oklch(0_0_0/0.45)] transition-[border-color,background-color] duration-150',
        'focus-visible:border-emerald focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-diamond',
        'disabled:cursor-not-allowed disabled:opacity-50',
        invalid
          ? 'border-danger bg-danger-soft/60'
          : 'border-line bg-void/70 hover:border-line-strong',
        className,
      ].join(' ')}
      {...props}
    />
  )
}
