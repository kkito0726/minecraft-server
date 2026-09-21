import type { ReactNode } from 'react'

/**
 * ラベル付きの選択欄。
 *
 * 選択肢が多いとき（Paper の版は 60 を超える）に使う。数個なら
 * ChoiceGroup で並べて見せる方が、既定を隠さずに済む。
 * 見た目は TextInput に揃える。
 */
export type SelectFieldProps = {
  id: string
  label: string
  value: string
  options: string[]
  onChange: (value: string) => void
  hint?: ReactNode | undefined
  disabled?: boolean | undefined
}

const SELECT_CLASS = [
  'w-full border px-2.5 py-2 text-sm text-fg',
  'shadow-[inset_0_2px_0_0_oklch(0_0_0/0.45)] transition-[border-color,background-color] duration-150',
  'focus-visible:border-emerald focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-diamond',
  'disabled:cursor-not-allowed disabled:opacity-50',
  'border-line bg-void/70 hover:border-line-strong',
].join(' ')

export function SelectField({ id, label, value, options, onChange, hint, disabled }: SelectFieldProps) {
  const hintId = hint ? `${id}-hint` : undefined

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-xs font-semibold tracking-wider text-dim">
        {label}
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        aria-describedby={hintId}
        className={SELECT_CLASS}
      >
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
      {hint && (
        <p id={hintId} className="text-xs text-faint">
          {hint}
        </p>
      )}
    </div>
  )
}
