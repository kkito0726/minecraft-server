import type { ReactNode } from 'react'

import { Radio } from '../atoms'

/**
 * 排他の選択肢。
 *
 * バックアップの方式と復元先はどちらも「選び間違えると
 * 取り返しがつかない」ので、既定を隠さず並べて見せる。
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
      <legend className="text-sm font-medium text-gray-800">{legend}</legend>
      {choices.map((choice) => (
        <label key={choice.value} className="flex items-start gap-2 text-sm">
          <Radio
            name={name}
            value={choice.value}
            checked={value === choice.value}
            disabled={disabled || choice.disabled}
            onChange={() => onChange(choice.value)}
            className="mt-0.5"
          />
          <span>
            <span className="text-gray-900">{choice.label}</span>
            {choice.hint && <span className="block text-xs text-gray-500">{choice.hint}</span>}
          </span>
        </label>
      ))}
    </fieldset>
  )
}
