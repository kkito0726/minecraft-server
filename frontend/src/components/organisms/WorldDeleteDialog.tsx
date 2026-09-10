import { useState } from 'react'

import { isConfirmed } from '../molecules/confirmation'
import { Button } from '../atoms'
import { ConfirmInput } from '../molecules'

/**
 * ワールドの削除の確認。
 *
 * 押し間違いがそのままデータの喪失になるため、対象の名前の
 * 完全一致を要求する（REQ-123）。バックエンドも同じ検証をするが、
 * ここで止めるのは「気づかせる」ためであって、守りは二重にする。
 *
 * 既定は退避（data/<名前>.deleted-<日時> へ mv）。完全な削除は
 * 退避の一覧から改めて選ぶ。
 */
export type WorldDeleteDialogProps = {
  name: string
  disabled?: boolean | undefined
  onCancel: () => void
  onConfirm: (confirmName: string) => void
}

export function WorldDeleteDialog({
  name,
  disabled,
  onCancel,
  onConfirm,
}: WorldDeleteDialogProps) {
  const [typed, setTyped] = useState('')
  const ready = isConfirmed(name, typed)

  return (
    <form
      className="flex flex-col gap-3 rounded border border-danger-500 bg-danger-50 p-3"
      aria-label={`${name} の削除`}
      onSubmit={(e) => {
        e.preventDefault()
        if (ready) {
          onConfirm(typed)
        }
      }}
    >
      <p className="text-sm text-danger-700">
        <strong className="font-semibold">{name}</strong> を削除します。
        すぐには消さず <code className="rounded bg-white px-1">{name}.deleted-日時</code>{' '}
        へ退避します。完全に消すには、退避の一覧から改めて削除してください。
      </p>

      <ConfirmInput
        id={`delete-${name}`}
        expected={name}
        value={typed}
        onChange={setTyped}
        disabled={disabled}
      />

      <div className="flex gap-2">
        <Button type="submit" tone="danger" disabled={!ready || disabled}>
          削除する
        </Button>
        <Button onClick={onCancel} disabled={disabled}>
          やめる
        </Button>
      </div>
    </form>
  )
}
