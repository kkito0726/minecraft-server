import type { ButtonHTMLAttributes, ReactNode } from 'react'

/**
 * ボタン。
 *
 * atoms は見た目だけを持ち、業務知識を持たない。
 * 「復元ボタン」ではなく「危険な操作の見た目のボタン」として振る舞う。
 */
export type ButtonTone = 'neutral' | 'primary' | 'danger'

const toneClasses: Record<ButtonTone, string> = {
  neutral: 'bg-white text-gray-800 border-gray-300 hover:bg-gray-50',
  primary: 'bg-gray-900 text-white border-gray-900 hover:bg-gray-700',
  danger: 'bg-danger-500 text-white border-danger-500 hover:bg-danger-700',
}

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: ButtonTone
  children: ReactNode
}

export function Button({ tone = 'neutral', className = '', ...props }: ButtonProps) {
  return (
    <button
      type="button"
      className={[
        'inline-flex items-center justify-center rounded border px-3 py-1.5',
        'text-sm font-medium transition-colors',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2',
        toneClasses[tone],
        className,
      ].join(' ')}
      {...props}
    />
  )
}
