import { isConfirmed } from './confirmation'
import { FormField } from './FormField'

/**
 * 対象の名称を打ち込ませる確認欄。
 *
 * 復元とワールド削除は取り消せない。「はい」を押すだけで実行できると、
 * 押し間違いがそのままデータの喪失になる。
 */
export type ConfirmInputProps = {
  id: string
  /** 打ち込ませる文字列。完全一致を要求する。 */
  expected: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean | undefined
}

export function ConfirmInput({ id, expected, value, onChange, disabled }: ConfirmInputProps) {
  const touched = value.length > 0
  const matches = isConfirmed(expected, value)

  return (
    <FormField
      id={id}
      label="確認のため、対象の名前を入力してください"
      value={value}
      onChange={onChange}
      hint={<code className="rounded bg-gray-100 px-1">{expected}</code>}
      error={touched && !matches ? '名前が一致していません' : undefined}
      placeholder={expected}
      disabled={disabled}
      autoFocus
    />
  )
}
