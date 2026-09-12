import { useState } from 'react'

import { parseRetention, retentionError } from '../../features/backups'
import type { RetentionInput } from '../../features/backups'
import type { RetentionPolicy } from '../../gen/mcadmin/v1/backup_pb'
import { Button } from '../atoms'
import { FormField } from '../molecules'

/**
 * 保持ポリシー。
 *
 * 世代数と日数の両方を満たさないものだけが削除され、設定がどうであれ
 * 最も新しい 1 世代は必ず残る（EDGE-103）。0 世代はバックアップの
 * 全損を意味するので、ここで止めて往復を省く（EDGE-104）。
 */
export type RetentionSettingsProps = {
  policy: RetentionPolicy
  disabled?: boolean | undefined
  onSave: (policy: RetentionInput) => void
}

export function RetentionSettings({ policy, disabled, onSave }: RetentionSettingsProps) {
  const [keepCount, setKeepCount] = useState(String(policy.keepCount))
  const [keepDays, setKeepDays] = useState(String(policy.keepDays))

  const error = retentionError(keepCount, keepDays)
  const changed =
    keepCount !== String(policy.keepCount) || keepDays !== String(policy.keepDays)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const parsed = parseRetention(keepCount, keepDays)
    if (parsed.ok) {
      onSave({ keepCount: parsed.keepCount, keepDays: parsed.keepDays })
    }
  }

  return (
    <form className="flex flex-col gap-4" aria-label="保持ポリシー" onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          id="retention-keep-count"
          label="残す世代数"
          value={keepCount}
          onChange={setKeepCount}
          disabled={disabled}
          hint="この数より古いものが削除の候補になります。"
        />
        <FormField
          id="retention-keep-days"
          label="保持日数"
          value={keepDays}
          onChange={setKeepDays}
          disabled={disabled}
          hint="0 で日数による削除を無効にします。"
        />
      </div>

      <p className="text-xs text-faint">
        世代数と日数の両方を満たさないものだけを削除します。
        設定がどうであれ、最も新しい 1 世代は必ず残します。
      </p>

      {error && (
        <p role="alert" className="text-sm text-danger-ink">
          {error}
        </p>
      )}

      <div>
        <Button type="submit" tone="primary" disabled={disabled || !changed || Boolean(error)}>
          保存する
        </Button>
      </div>
    </form>
  )
}
