import type { ReactNode } from 'react'

import { Radio } from '../atoms'

/**
 * 排他の選択肢。
 *
 * バックアップの方式と復元先はどちらも「選び間違えると
 * 取り返しがつかない」ので、既定を隠さず並べて見せる。
 * 選択肢ごとに枠を持たせ、どれが選ばれているかを離れて見ても分かるようにする。
 */
export type Choice<T extends string> = {
  value: T
  label: string
  /** 選ぶ前に知っておくべきこと。ラベルの下に小さく出す。 */
  hint?: ReactNode | undefined
  disabled?: boolean | undefined
}

export type ChoiceGroupProps<T extends string> = {
  name: string
  legend: string
  value: T
  choices: Choice<T>[]
  onChange: (value: T) => void
  disabled?: boolean | undefined
}

const CHOICE_CLASS = [
  'flex cursor-pointer items-start gap-3 border px-3 py-2.5 text-sm transition-colors',
  'border-line bg-void/40 hover:border-line-strong',
  'has-checked:border-emerald/60 has-checked:bg-emerald-soft/60',
  'has-disabled:cursor-not-allowed has-disabled:opacity-50',
].join(' ')

export function ChoiceGroup<T extends string>({
  name,
  legend,
  value,
  choices,
  onChange,
  disabled,
}: ChoiceGroupProps<T>) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-xs font-semibold tracking-wider text-dim">{legend}</legend>
      {choices.map((choice) => (
        <label key={choice.value} className={CHOICE_CLASS}>
          <Radio
            name={name}
            value={choice.value}
            checked={value === choice.value}
            disabled={disabled || choice.disabled}
            onChange={() => onChange(choice.value)}
            className="mt-0.5"
          />
          <span>
            <span className="font-semibold text-fg">{choice.label}</span>
            {choice.hint && <span className="mt-0.5 block text-xs text-faint">{choice.hint}</span>}
          </span>
        </label>
      ))}
    </fieldset>
  )
}
