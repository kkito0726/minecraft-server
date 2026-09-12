import { useState } from 'react'

import { isValidWorldName, worldNameError } from '../../features/worlds'
import { Button } from '../atoms'
import { FormField } from '../molecules'

/**
 * ワールド名を入力させる小さなダイアログ。
 *
 * 作成・複製・改名で共通。名前の規則はバックエンドと同一の
 * 正規表現で検証する（NFR-304）。ここで弾くのは往復を減らすためで、
 * 守りはバックエンドの値オブジェクトが担う。
 */
export type WorldNameDialogProps = {
  title: string
  /** 入力欄の説明。何を作ろうとしているかを示す。 */
  description: string
  submitLabel: string
  /** シードの入力欄を出すか。新規作成のときだけ。 */
  withSeed?: boolean | undefined
  disabled?: boolean | undefined
  onCancel: () => void
  onSubmit: (name: string, seed: string) => void
}

export function WorldNameDialog({
  title,
  description,
  submitLabel,
  withSeed,
  disabled,
  onCancel,
  onSubmit,
}: WorldNameDialogProps) {
  const [name, setName] = useState('')
  const [seed, setSeed] = useState('')
  const ready = isValidWorldName(name)

  return (
    <form
      className="hud-inset flex flex-col gap-4"
      aria-label={title}
      onSubmit={(e) => {
        e.preventDefault()
        if (ready) {
          onSubmit(name, seed)
        }
      }}
    >
      <p className="text-sm text-dim">{description}</p>

      <NameField value={name} onChange={setName} disabled={disabled} />

      {withSeed && <SeedField value={seed} onChange={setSeed} disabled={disabled} />}

      <div className="flex gap-2">
        <Button type="submit" tone="primary" disabled={!ready || disabled}>
          {submitLabel}
        </Button>
        <Button onClick={onCancel} disabled={disabled}>
          やめる
        </Button>
      </div>
    </form>
  )
}

type FieldProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean | undefined
}

function NameField({ value, onChange, disabled }: FieldProps) {
  return (
    <FormField
      id="world-name"
      label="ワールド名"
      value={value}
      onChange={onChange}
      error={worldNameError(value)}
      hint="英数字で始まる 1〜32 文字（英数字・_・-）"
      disabled={disabled}
      autoFocus
    />
  )
}

function SeedField({ value, onChange, disabled }: FieldProps) {
  return (
    <FormField
      id="world-seed"
      label="シード（任意）"
      value={value}
      onChange={onChange}
      hint="空にすると Paper が決めます"
      disabled={disabled}
    />
  )
}
