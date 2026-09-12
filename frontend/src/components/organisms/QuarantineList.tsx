import { formatBytes, formatDateTime } from '../../lib/format'
import type { Quarantine } from '../../gen/mcadmin/v1/world_pb'
import { Badge, Button } from '../atoms'
import { PanelHeader } from '../molecules'

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
    <section className="hud-panel flex flex-col gap-4">
      <div>
        <PanelHeader title="退避したワールド" tag="QUARANTINE" />
        <p className="mt-2 text-xs text-faint">
          復元や削除の前に退避したものです。自動では消しません。
          問題なく動くことを確認してから削除してください。
        </p>
      </div>

      <ul className="flex flex-col">
        {quarantines.map((q) => (
          <li
            key={q.name}
            className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line/60 py-2.5 text-sm last:border-b-0"
          >
            <code className="code-chip">{q.name}</code>
            <Badge tone="neutral">{q.fromRestore ? '復元による退避' : '削除による退避'}</Badge>
            <span className="text-dim tabular-nums">{formatBytes(q.sizeBytes)}</span>
            <span className="text-dim tabular-nums">{formatDateTime(q.quarantinedAt?.seconds)}</span>
            <div className="ml-auto">
              <Button tone="danger" size="sm" disabled={disabled} onClick={() => onPurge(q.name)}>
                完全に削除
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
