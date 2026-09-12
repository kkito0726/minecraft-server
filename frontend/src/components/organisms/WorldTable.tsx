import { formatWorldVersion } from '../../features/worlds'
import { formatBytes, formatDateTime } from '../../lib/format'
import type { World } from '../../gen/mcadmin/v1/world_pb'
import { Badge, Button, PixelIcon } from '../atoms'

/**
 * ワールドの一覧。
 *
 * 稼働中のものには印を付ける。切替はサーバーの再作成を伴うため、
 * 稼働中のワールドには切替ボタンを出さない。
 */
export type WorldTableProps = {
  worlds: World[]
  disabled: boolean
  onSwitch: (name: string) => void
  onClone: (name: string) => void
  onRename: (name: string) => void
  onDelete: (name: string) => void
}

export function WorldTable(props: WorldTableProps) {
  if (props.worlds.length === 0) {
    return <p className="text-sm text-dim">ワールドがありません。</p>
  }

  return (
    <div className="overflow-x-auto">
      <table className="hud-table">
        <thead>
          <tr>
            <th>名前</th>
            <th>バージョン</th>
            <th>大きさ</th>
            <th>最終プレイ</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {props.worlds.map((world) => (
            <WorldRow key={world.name} world={world} {...props} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

type WorldRowProps = WorldTableProps & { world: World }

function WorldRow({ world, disabled, onSwitch, onClone, onRename, onDelete }: WorldRowProps) {
  return (
    <tr data-active={world.active || undefined}>
      <td>
        <div className="flex items-center gap-2.5">
          <PixelIcon
            name="world"
            className={`size-5 shrink-0 ${world.active ? 'text-emerald' : 'text-faint'}`}
          />
          <span className="font-semibold text-fg">{world.name}</span>
          {world.active && <Badge tone="ok">稼働中</Badge>}
        </div>
      </td>
      <td>{formatWorldVersion(world.version)}</td>
      <td className="whitespace-nowrap">{formatBytes(world.sizeBytes)}</td>
      <td className="whitespace-nowrap">{formatDateTime(world.lastPlayed?.seconds) || '—'}</td>
      <td>
        <div className="flex gap-1.5">
          {!world.active && (
            <Button size="sm" disabled={disabled} onClick={() => onSwitch(world.name)}>
              切替
            </Button>
          )}
          <Button size="sm" disabled={disabled} onClick={() => onClone(world.name)}>
            複製
          </Button>
          <Button size="sm" disabled={disabled} onClick={() => onRename(world.name)}>
            改名
          </Button>
          {/* 稼働中のワールドは削除できない。先に切り替えてもらう。 */}
          <Button
            tone="danger"
            size="sm"
            disabled={disabled || world.active}
            onClick={() => onDelete(world.name)}
          >
            削除
          </Button>
        </div>
      </td>
    </tr>
  )
}
