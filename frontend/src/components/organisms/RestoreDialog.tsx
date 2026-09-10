import { useState } from 'react'

import { targetLabel, verdictLabel, verdictTone } from '../../features/backups'
import { formatWorldVersion } from '../../features/worlds'
import { RestoreTarget } from '../../gen/mcadmin/v1/backup_pb'
import type { PreflightRestoreResponse } from '../../gen/mcadmin/v1/backup_pb'
import { formatBytes } from '../../lib/format'
import { Badge, Button } from '../atoms'
import { CheckboxField, ChoiceGroup, ConfirmInput, isConfirmed } from '../molecules'

/**
 * 復元の確認（REQ-109 / REQ-112 / REQ-123）。
 *
 * 復元は取り消せない。関門を 2 つ置く。
 *
 *  1. 復元先のワールド名の完全一致
 *  2. バージョンが食い違うときは、警告を読んだという承諾
 *
 * バックエンドは同じ 2 つを独立に検証する。ここで止めるのは
 * 「気づかせる」ためであって、守りとしては二重にしてある。
 */
export type RestoreRequestInput = {
  target: RestoreTarget
  acknowledgeVersionWarning: boolean
  confirmLevelName: string
}

export type RestoreDialogProps = {
  preflight: PreflightRestoreResponse
  disabled?: boolean | undefined
  onCancel: () => void
  onConfirm: (input: RestoreRequestInput) => void
}

type TargetKey = 'archive' | 'current'

const TARGETS: Record<TargetKey, RestoreTarget> = {
  archive: RestoreTarget.ARCHIVE_LEVEL,
  current: RestoreTarget.CURRENT_LEVEL,
}

export function RestoreDialog({ preflight, disabled, onCancel, onConfirm }: RestoreDialogProps) {
  const archiveLevel = preflight.backup?.archiveLevel ?? ''
  const {
    target,
    typed,
    setTyped,
    acknowledged,
    setAcknowledged,
    expected,
    ready,
    changeTarget,
  } = useRestoreConfirmation(preflight, archiveLevel)

  return (
    <form
      className="flex flex-col gap-3 rounded border border-danger-500 bg-danger-50 p-3"
      aria-label="バックアップからの復元"
      onSubmit={(e) => {
        e.preventDefault()
        if (ready) {
          onConfirm({
            target: TARGETS[target],
            acknowledgeVersionWarning: acknowledged,
            confirmLevelName: typed,
          })
        }
      }}
    >
      <RestoreSummary preflight={preflight} />

      <TargetChoice
        archiveLevel={archiveLevel}
        currentLevel={preflight.currentLevel}
        value={target}
        disabled={disabled}
        onChange={changeTarget}
      />

      <SpaceNotice required={preflight.requiredBytes} available={preflight.availableBytes} />

      <ConfirmSection
        expected={expected}
        typed={typed}
        onTyped={setTyped}
        needsAcknowledgement={preflight.requiresConfirmation}
        acknowledged={acknowledged}
        onAcknowledged={setAcknowledged}
        disabled={disabled}
      />

      <DialogActions ready={ready} disabled={disabled} onCancel={onCancel} />
    </form>
  )
}

function DialogActions({
  ready,
  disabled,
  onCancel,
}: {
  ready: boolean
  disabled: boolean | undefined
  onCancel: () => void
}) {
  return (
    <div className="flex gap-2">
      <Button type="submit" tone="danger" disabled={!ready || disabled}>
        復元する
      </Button>
      <Button onClick={onCancel} disabled={disabled}>
        やめる
      </Button>
    </div>
  )
}

/**
 * 2 つの関門。名前の完全一致と、警告への承諾。
 *
 * 承諾のチェックは必要なときだけ出す。常に出すと押す癖がついて
 * 「読んだ」という意味を失う。
 */
function ConfirmSection({
  expected,
  typed,
  onTyped,
  needsAcknowledgement,
  acknowledged,
  onAcknowledged,
  disabled,
}: {
  expected: string
  typed: string
  onTyped: (value: string) => void
  needsAcknowledgement: boolean
  acknowledged: boolean
  onAcknowledged: (value: boolean) => void
  disabled: boolean | undefined
}) {
  return (
    <>
      <ConfirmInput
        id="restore-confirm"
        expected={expected}
        value={typed}
        onChange={onTyped}
        disabled={disabled}
      />
      {needsAcknowledgement && (
        <CheckboxField
          id="restore-acknowledge"
          label="上の警告を読み、そのうえで復元します。"
          checked={acknowledged}
          onChange={onAcknowledged}
          disabled={disabled}
        />
      )}
    </>
  )
}

/**
 * 復元先の選択（REQ-112）。
 *
 * アーカイブのワールド名を読み取れなかった場合、その選択肢は
 * サーバー側でも拒否される。押せるように見せない。
 */
function TargetChoice({
  archiveLevel,
  currentLevel,
  value,
  disabled,
  onChange,
}: {
  archiveLevel: string
  currentLevel: string
  value: TargetKey
  disabled: boolean | undefined
  onChange: (value: TargetKey) => void
}) {
  return (
    <ChoiceGroup
      name="restore-target"
      legend="どのワールド名として復元しますか"
      value={value}
      disabled={disabled}
      onChange={onChange}
      choices={[
        {
          value: 'archive',
          label: targetLabel(RestoreTarget.ARCHIVE_LEVEL),
          hint: archiveLevel
            ? `data/${archiveLevel}/ に戻し、MC_LEVEL も ${archiveLevel} に切り替えます。`
            : 'アーカイブのワールド名を読み取れませんでした。',
          disabled: archiveLevel === '',
        },
        {
          value: 'current',
          label: targetLabel(RestoreTarget.CURRENT_LEVEL),
          hint: `展開時にパスを書き換え、稼働中の ${currentLevel} として戻します。`,
        },
      ]}
    />
  )
}

/**
 * 2 つの関門の状態。
 *
 * 復元先を変えると打ち込むべき名前も変わる。入力が残っていると
 * 「合っているのに押せない」状態になり、理由が画面から読めない。
 */
function useRestoreConfirmation(preflight: PreflightRestoreResponse, archiveLevel: string) {
  const [target, setTarget] = useState<TargetKey>(archiveLevel ? 'archive' : 'current')
  const [typed, setTyped] = useState('')
  const [acknowledged, setAcknowledged] = useState(false)

  const expected = target === 'archive' ? archiveLevel : preflight.currentLevel

  return {
    target,
    typed,
    setTyped,
    acknowledged,
    setAcknowledged,
    expected,
    ready:
      expected !== '' &&
      isConfirmed(expected, typed) &&
      (!preflight.requiresConfirmation || acknowledged),
    changeTarget: (next: TargetKey) => {
      setTarget(next)
      setTyped('')
    },
  }
}

/** 何が起きるかと、バージョンの判定。承諾を求める根拠になる部分。 */
function RestoreSummary({ preflight }: { preflight: PreflightRestoreResponse }) {
  return (
    <div className="flex flex-col gap-2 text-sm text-danger-700">
      <p>
        <code className="rounded bg-white px-1">{preflight.backup?.id}</code> から復元します。
        現在のワールドは削除せず <code className="rounded bg-white px-1">.broken-日時</code>{' '}
        へ退避してから展開します。うまくいかなければ退避から戻せます。
      </p>

      <p className="flex flex-wrap items-center gap-2">
        <Badge tone={verdictTone(preflight.verdict)}>{verdictLabel(preflight.verdict)}</Badge>
        <span className="text-gray-700">
          アーカイブ {formatWorldVersion(preflight.backup?.version)} / 現在{' '}
          {formatWorldVersion(preflight.currentWorldVersion)}
        </span>
      </p>

      {preflight.levelNameMismatch && (
        <p className="text-gray-700">
          アーカイブのワールド名は{' '}
          <code className="rounded bg-white px-1">{preflight.backup?.archiveLevel || '不明'}</code>{' '}
          で、稼働中の <code className="rounded bg-white px-1">{preflight.currentLevel}</code>{' '}
          と違います。そのまま戻すと、稼働中のワールドは何も変わりません。
        </p>
      )}

      {preflight.warnings.length > 0 && (
        <ul className="list-disc pl-5">
          {preflight.warnings.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

/**
 * 空き容量。足りなければ手順 2 で失敗する。
 * サーバーを止める前に分かるのだから、押す前に見せる。
 */
function SpaceNotice({ required, available }: { required: bigint; available: bigint }) {
  if (available >= required) {
    return null
  }

  return (
    <p role="alert" className="rounded border border-danger-500 bg-white p-2 text-sm text-danger-700">
      空き容量が足りません。展開には {formatBytes(required)} 必要ですが、空きは{' '}
      {formatBytes(available)} です。
    </p>
  )
}
