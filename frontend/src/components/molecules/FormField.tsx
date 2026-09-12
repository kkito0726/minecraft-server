import type { ReactNode } from 'react'

import { TextInput } from '../atoms'

/**
 * ラベル・入力・説明・エラーをひとまとめにした入力欄。
 *
 * atoms の TextInput は見た目だけを持つので、ラベルとの紐付けや
 * エラーの読み上げはここで組み立てる。
 */
export type FormFieldProps = {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  /** 入力の下に出す説明。制約を先に伝えるために使う。 */
  hint?: ReactNode | undefined
  /** 検証に失敗した理由。設定すると入力欄が異常表示になる。 */
  error?: string | undefined
  placeholder?: string | undefined
  disabled?: boolean | undefined
  autoFocus?: boolean | undefined
}

export function FormField({
  id,
  label,
  value,
  onChange,
  hint,
  error,
  placeholder,
  disabled,
  autoFocus,
}: FormFieldProps) {
  const hintId = hint ? `${id}-hint` : undefined
  const errorId = error ? `${id}-error` : undefined

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-xs font-semibold tracking-wider text-dim">
        {label}
      </label>
      <TextInput
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        invalid={Boolean(error)}
        placeholder={placeholder}
        disabled={disabled}
        autoFocus={autoFocus}
        aria-describedby={[hintId, errorId].filter(Boolean).join(' ') || undefined}
      />
      {hint && (
        <p id={hintId} className="text-xs text-faint">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="text-xs font-medium text-danger-ink">
          {error}
        </p>
      )}
    </div>
  )
}
