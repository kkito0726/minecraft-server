import { useState } from 'react'

import { formatWorldVersion } from '../../features/worlds'
import type { Backup, ListBackupsResponse } from '../../gen/mcadmin/v1/backup_pb'
import { formatBytes, formatDateTime } from '../../lib/format'
import { Button } from '../atoms'

/**
 * バックアップの一覧。
 *
 * 表示するバージョンはアーカイブ内の level.dat から読んだ値。
 * ファイル名の版は改名できるので信用しない（REQ-413）。
 */
export type BackupTableProps = {
  data: ListBackupsResponse
  disabled: boolean
  onRestore: (id: string) => void
  onDelete: (id: string) => void
}

export function BackupTable({ data, disabled, onRestore, onDelete }: BackupTableProps) {
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)

  return (
    <div className="flex flex-col gap-3">
      {data.backups.length === 0 ? (
        <p className="text-sm text-gray-600">
          まだバックアップがありません。「バックアップを取得」から作れます。
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 text-left text-xs text-gray-500">
                <th className="py-2 pr-2 font-medium">ファイル</th>
                <th className="py-2 pr-2 font-medium">バージョン</th>
                <th className="py-2 pr-2 font-medium">ワールド</th>
                <th className="py-2 pr-2 font-medium">大きさ</th>
                <th className="py-2 pr-2 font-medium">取得日時</th>
                <th className="py-2 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {data.backups.map((backup) => (
                <BackupRow
                  key={backup.id}
                  backup={backup}
                  disabled={disabled}
                  confirming={pendingDelete === backup.id}
                  onRestore={onRestore}
                  onDelete={onDelete}
                  onAskDelete={setPendingDelete}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <StorageNotice data={data} />
    </div>
  )
}

type BackupRowProps = {
  backup: Backup
  disabled: boolean
  confirming: boolean
  onRestore: (id: string) => void
  onDelete: (id: string) => void
  onAskDelete: (id: string | null) => void
}

function BackupRow({
  backup,
  disabled,
  confirming,
  onRestore,
  onDelete,
  onAskDelete,
}: BackupRowProps) {
  return (
    <tr className="border-b border-gray-100">
      <td className="py-2 pr-2">
        <code className="text-gray-900">{backup.id}</code>
      </td>
      <td className="py-2 pr-2 text-gray-700">{formatWorldVersion(backup.version)}</td>
      <td className="py-2 pr-2 text-gray-700">{backup.archiveLevel || '不明'}</td>
      <td className="py-2 pr-2 text-gray-700">{formatBytes(backup.sizeBytes)}</td>
      <td className="py-2 pr-2 text-gray-700">
        {formatDateTime(backup.createdAt?.seconds) || '—'}
      </td>
      <td className="py-2">
        <RowActions
          backup={backup}
          disabled={disabled}
          confirming={confirming}
          onRestore={onRestore}
          onDelete={onDelete}
          onAskDelete={onAskDelete}
        />
      </td>
    </tr>
  )
}

function RowActions({
  backup,
  disabled,
  confirming,
  onRestore,
  onDelete,
  onAskDelete,
}: BackupRowProps) {
  return (
    <div className="flex gap-1">
      <Button disabled={disabled} onClick={() => onRestore(backup.id)}>
        復元
      </Button>
      {/* 削除は取り消せない。一度で消えないよう二度押しさせる。 */}
      {confirming ? (
        <>
          <Button
            tone="danger"
            disabled={disabled}
            onClick={() => {
              onAskDelete(null)
              onDelete(backup.id)
            }}
          >
            本当に削除
          </Button>
          <Button disabled={disabled} onClick={() => onAskDelete(null)}>
            やめる
          </Button>
        </>
      ) : (
        <Button tone="danger" disabled={disabled} onClick={() => onAskDelete(backup.id)}>
          削除
        </Button>
      )}
    </div>
  )
}

/**
 * v1 はオフサイト転送を持たない（スコープ外）。ディスクが壊れれば
 * バックアップも一緒に失われる。一覧には必ずこれを添える。
 */
function StorageNotice({ data }: { data: ListBackupsResponse }) {
  return (
    <p className="text-xs text-gray-500">
      保管先 <code className="rounded bg-gray-100 px-1">{data.directory}</code>（合計{' '}
      {formatBytes(data.totalSizeBytes)}）。
      バックアップはこのディスク上にしかありません。ディスクが壊れると一緒に失われます。
      大事な世代は別の場所へも控えてください。
    </p>
  )
}
