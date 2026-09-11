import { useState } from 'react'

import {
  BackupTable,
  CreateBackupDialog,
  PrunePanel,
  RestoreDialog,
  RetentionSettings,
  UploadBackupPanel,
} from '../components/organisms'
import type { PruneResult, RestoreRequestInput } from '../components/organisms'
import { Button } from '../components/atoms'
import {
  useBackups,
  useCreateBackup,
  useDeleteBackup,
  usePreflightRestore,
  usePruneBackups,
  useRestoreBackup,
  useRetentionPolicy,
  useSetRetentionPolicy,
  uploadNotice,
  useUploadBackup,
} from '../features/backups'
import { useOperation } from '../features/operations'
import type { ListBackupsResponse, RetentionPolicy } from '../gen/mcadmin/v1/backup_pb'
import { describeError } from '../lib/errors'

/**
 * バックアップの取得・一覧・復元と保持設定（REQ-101〜REQ-116）。
 *
 * 復元だけが 2 段になっている。事前確認（副作用なし）を取り直してから
 * 確認のダイアログを出し、承諾がそろってはじめて操作を始める。
 */
export function BackupsPage() {
  const { isBusy } = useOperation()
  const backups = useBackups()
  const retention = useRetentionPolicy()

  if (backups.isPending) {
    return <p className="text-sm text-gray-600">バックアップを読み込んでいます…</p>
  }
  if (backups.error) {
    return <p className="text-sm text-danger-700">{describeError(backups.error)}</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <BackupSection data={backups.data} disabled={isBusy} />
      <RetentionSection disabled={isBusy} policy={retention.data?.policy} />
    </div>
  )
}

function BackupSection({ data, disabled }: { data: ListBackupsResponse; disabled: boolean }) {
  const [restoring, setRestoring] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [importing, setImporting] = useState(false)
  const { create, remove, restore, preflight, failure } = useBackupActions(restoring)
  const upload = useUploadBackup()

  const send = (input: RestoreRequestInput) => {
    if (restoring !== null) {
      restore.mutate({ backupId: restoring, ...input })
    }
    setRestoring(null)
  }

  return (
    <section className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4">
      <Header
        disabled={disabled}
        onCreate={() => setCreating(true)}
        onImport={() => setImporting((v) => !v)}
      />

      {failure && <p className="text-sm text-danger-700">{describeError(failure)}</p>}

      {creating && (
        <CreateBackupDialog
          disabled={disabled}
          onCancel={() => setCreating(false)}
          onSubmit={(input) => {
            create.mutate(input)
            setCreating(false)
          }}
        />
      )}

      {importing && <ImportSection disabled={disabled} upload={upload} />}

      {restoring !== null && (
        <RestoreSection
          pending={preflight.isPending}
          preflight={preflight.data}
          disabled={disabled}
          onCancel={() => setRestoring(null)}
          onConfirm={send}
        />
      )}

      <BackupTable
        data={data}
        disabled={disabled}
        onRestore={setRestoring}
        onDelete={(id) => remove.mutate(id)}
      />
    </section>
  )
}

/** 取り込みの状態をそのまま画面へ渡すだけの継ぎ手。 */
function ImportSection({
  disabled,
  upload,
}: {
  disabled: boolean
  upload: ReturnType<typeof useUploadBackup>
}) {
  return (
    <UploadBackupPanel
      disabled={disabled}
      progress={upload.progress}
      error={upload.error?.message}
      notice={uploadNotice(upload.data)}
      onUpload={(file) => upload.mutate(file)}
    />
  )
}

/**
 * 一覧に対する操作をまとめる。
 *
 * 事前確認は復元先の id が決まっているときだけ走る。副作用を
 * 持たないので、ダイアログを開くたびに取り直して構わない（REQ-008）。
 */
function useBackupActions(restoring: string | null) {
  const create = useCreateBackup()
  const remove = useDeleteBackup()
  const restore = useRestoreBackup()
  const preflight = usePreflightRestore(restoring)

  return {
    create,
    remove,
    restore,
    preflight,
    failure: create.error ?? remove.error ?? restore.error ?? preflight.error,
  }
}

type HeaderProps = {
  disabled: boolean
  onCreate: () => void
  onImport: () => void
}

function Header({ disabled, onCreate, onImport }: HeaderProps) {
  return (
    <div className="flex items-center gap-2">
      <h2 className="text-sm font-semibold text-gray-900">バックアップ</h2>
      <div className="ml-auto flex gap-2">
        <Button disabled={disabled} onClick={onImport}>
          zip を取り込む
        </Button>
        <Button tone="primary" disabled={disabled} onClick={onCreate}>
          バックアップを取得
        </Button>
      </div>
    </div>
  )
}

type RestoreSectionProps = {
  pending: boolean
  preflight: Parameters<typeof RestoreDialog>[0]['preflight'] | undefined
  disabled: boolean
  onCancel: () => void
  onConfirm: (input: RestoreRequestInput) => void
}

/**
 * 事前確認が取れるまでは何も選ばせない。
 * 古い判定のまま承諾させると、承諾の意味が無くなる。
 */
function RestoreSection({ pending, preflight, disabled, onCancel, onConfirm }: RestoreSectionProps) {
  if (pending || !preflight) {
    return <p className="text-sm text-gray-600">復元の内容を確認しています…</p>
  }
  return (
    <RestoreDialog
      preflight={preflight}
      disabled={disabled}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  )
}

function RetentionSection({
  disabled,
  policy,
}: {
  disabled: boolean
  policy: RetentionPolicy | undefined
}) {
  const save = useSetRetentionPolicy()
  const prune = usePruneBackups()
  const [pruned, setPruned] = useState<PruneResult | null>(null)

  if (!policy) {
    return null
  }

  return (
    <section className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-gray-900">保持ポリシー</h2>

      {(save.error ?? prune.error) && (
        <p className="text-sm text-danger-700">{describeError(save.error ?? prune.error)}</p>
      )}

      <RetentionSettings
        policy={policy}
        disabled={disabled || save.isPending}
        onSave={(next) => save.mutate(next)}
      />

      {/*
        消したあとは予定ではなく実際に消えたものを出す。確認のあとに
        取得が走れば保持ポリシーが先に適用され、対象は変わりうる。
      */}
      <PrunePanel
        result={pruned}
        disabled={disabled || prune.isPending}
        onPreview={() =>
          prune.mutate(true, {
            onSuccess: (res) => setPruned({ kind: 'preview', ids: res.deletedIds }),
          })
        }
        onApply={() =>
          prune.mutate(false, {
            onSuccess: (res) => setPruned({ kind: 'applied', ids: res.deletedIds }),
          })
        }
      />
    </section>
  )
}
