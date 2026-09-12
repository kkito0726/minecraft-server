import type { ButtonHTMLAttributes, ReactNode } from 'react'

/**
 * ボタン。
 *
 * atoms は見た目だけを持ち、業務知識を持たない。
 * 「復元ボタン」ではなく「危険な操作の見た目のボタン」として振る舞う。
 * 見た目の実体は styles/index.css の .btn にある。面取りと押し込みの
 * 影を状態ごとに持つため、クラスを並べるより CSS にまとめたほうが読める。
 */
export type ButtonTone = 'neutral' | 'primary' | 'danger'

/** 表の行の中では小さく、画面の主操作では大きく。 */
export type ButtonSize = 'sm' | 'md' | 'lg'

const toneClasses: Record<ButtonTone, string> = {
  neutral: 'btn-neutral',
  primary: 'btn-primary',
  danger: 'btn-danger',
}

const sizeClasses: Record<ButtonSize, string> = {
  sm: 'btn-sm',
  md: '',
  lg: 'btn-lg',
}

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: ButtonTone
  size?: ButtonSize
  children: ReactNode
}

export function Button({ tone = 'neutral', size = 'md', className = '', ...props }: ButtonProps) {
  return (
    <button
      type="button"
      className={['btn', toneClasses[tone], sizeClasses[size], className]
        .filter(Boolean)
        .join(' ')}
      {...props}
    />
  )
}
