import { useState } from 'react'

import { MAX_NOTE_LENGTH, modeLabel, noteSlug } from '../../features/backups'
import type { CreateBackupInput } from '../../features/backups'
import { BackupMode } from '../../gen/mcadmin/v1/backup_pb'
import { Button } from '../atoms'
import { ChoiceGroup, FormField } from '../molecules'

/**
 * バックアップの取得。
 *
 * 既定は HOT（稼働したまま）。停止を伴う COLD を既定にすると、
 * 軽い気持ちのバックアップがそのままサーバーの再起動になる。
 */
export type CreateBackupDialogProps = {
  disabled?: boolean | undefined
  onCancel: () => void
  onSubmit: (input: CreateBackupInput) => void
}

type ModeKey = 'hot' | 'cold'

const MODES: Record<ModeKey, BackupMode> = {
  hot: BackupMode.HOT,
  cold: BackupMode.COLD,
}

export function CreateBackupDialog({ disabled, onCancel, onSubmit }: CreateBackupDialogProps) {
  const [mode, setMode] = useState<ModeKey>('hot')
  const [note, setNote] = useState('')

  return (
    <form
      className="flex flex-col gap-3 rounded border border-gray-300 bg-gray-50 p-3"
      aria-label="バックアップの取得"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ mode: MODES[mode], note })
      }}
    >
      <ChoiceGroup
        name="backup-mode"
        legend="取得の方式"
        value={mode}
        disabled={disabled}
        onChange={setMode}
        choices={[
          { value: 'hot', label: modeLabel(BackupMode.HOT), hint: HOT_HINT },
          { value: 'cold', label: modeLabel(BackupMode.COLD), hint: COLD_HINT },
        ]}
      />

      {mode === 'cold' && (
        <p role="alert" className="rounded border border-warn-500 bg-warn-50 p-2 text-sm text-warn-700">
          サーバーを停止してから取得し、終わったら起動し直します。接続中の人は切断されます。
        </p>
      )}

      <NoteField note={note} disabled={disabled} onChange={setNote} />

      <div className="flex gap-2">
        <Button type="submit" tone="primary" disabled={disabled}>
          取得する
        </Button>
        <Button onClick={onCancel} disabled={disabled}>
          やめる
        </Button>
      </div>
    </form>
  )
}

const HOT_HINT = '保存を一時的に止めてから固めます。サーバーは動いたままです。'
const COLD_HINT = '確実ですが、その間サーバーは止まります。'

/**
 * メモはファイル名の一部になる。使えない文字は落ちるため、
 * 押したあとではなく打っている最中に結果を見せる。
 */
function NoteField({
  note,
  disabled,
  onChange,
}: {
  note: string
  disabled: boolean | undefined
  onChange: (value: string) => void
}) {
  const slug = noteSlug(note)
  const dropped = note.trim() !== '' && slug === ''

  return (
    <FormField
      id="backup-note"
      label="メモ（任意）"
      value={note}
      onChange={onChange}
      disabled={disabled}
      placeholder="before-update"
      hint={
        dropped ? (
          <span className="text-warn-700">
            このメモはファイル名には残りません。英数字と下線だけが使えます。
          </span>
        ) : slug ? (
          <>
            ファイル名の末尾に <code className="rounded bg-gray-100 px-1">{slug}</code> が付きます。
          </>
        ) : (
          `英数字と下線が ${MAX_NOTE_LENGTH} 文字まで使えます。`
        )
      }
    />
  )
}
