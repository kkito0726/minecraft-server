import type { ReactNode } from 'react'

import { Checkbox } from '../atoms'

/**
 * 承諾のチェック。
 *
 * 「読んだうえで進む」ことを一手間として要求するために使う。
 * 常に出すと押す癖がつくので、必要なときだけ出す側で判断する。
 */
export type CheckboxFieldProps = {
  id: string
  label: ReactNode
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean | undefined
}

export function CheckboxField({ id, label, checked, onChange, disabled }: CheckboxFieldProps) {
  return (
    <label htmlFor={id} className="flex items-start gap-2 text-sm text-gray-800">
      <Checkbox
        id={id}
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="mt-0.5"
      />
      <span>{label}</span>
    </label>
  )
}
