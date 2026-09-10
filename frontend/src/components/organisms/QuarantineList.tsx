import { formatBytes, formatDateTime } from '../../lib/format'
import type { Quarantine } from '../../gen/mcadmin/v1/world_pb'
import { Badge, Button } from '../atoms'

/**
 * 退避されたディレクトリの一覧。
 *
 * システムは自動で削除しない（REQ-116）。docs の「問題なく動くことを
 * 確認してから削除する」を画面に落としたもの。ここを自動化すると、
 * 復元がうまくいかなかったときに戻す先が無くなる。
 */
export type QuarantineListProps = {
  quarantines: Quarantine[]
  disabled: boolean
  onPurge: (name: string) => void
}

export function QuarantineList({ quarantines, disabled, onPurge }: QuarantineListProps) {
  if (quarantines.length === 0) {
    return null
  }

  return (
    <section className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4">
      <div>
        <h2 className="text-sm font-semibold text-gray-900">退避したワールド</h2>
        <p className="mt-1 text-xs text-gray-500">
          復元や削除の前に退避したものです。自動では消しません。
          問題なく動くことを確認してから削除してください。
        </p>
      </div>

      <ul className="flex flex-col gap-2">
        {quarantines.map((q) => (
          <li
            key={q.name}
            className="flex flex-wrap items-center gap-2 border-b border-gray-100 pb-2 text-sm"
          >
            <code className="text-gray-900">{q.name}</code>
            <Badge tone="neutral">{q.fromRestore ? '復元による退避' : '削除による退避'}</Badge>
            <span className="text-gray-600">{formatBytes(q.sizeBytes)}</span>
            <span className="text-gray-600">{formatDateTime(q.quarantinedAt?.seconds)}</span>
            <div className="ml-auto">
              <Button tone="danger" disabled={disabled} onClick={() => onPurge(q.name)}>
                完全に削除
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
