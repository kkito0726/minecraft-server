import { formatWorldVersion } from '../../features/worlds'
import { formatBytes, formatDateTime } from '../../lib/format'
import type { World } from '../../gen/mcadmin/v1/world_pb'
import { Badge, Button } from '../atoms'

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
    return <p className="text-sm text-gray-600">ワールドがありません。</p>
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-200 text-left text-xs text-gray-500">
            <th className="py-2 pr-2 font-medium">名前</th>
            <th className="py-2 pr-2 font-medium">バージョン</th>
            <th className="py-2 pr-2 font-medium">大きさ</th>
            <th className="py-2 pr-2 font-medium">最終プレイ</th>
            <th className="py-2 font-medium">操作</th>
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
    <tr className="border-b border-gray-100">
      <td className="py-2 pr-2">
        <span className="font-medium text-gray-900">{world.name}</span>
        {world.active && (
          <span className="ml-2">
            <Badge tone="ok">稼働中</Badge>
          </span>
        )}
      </td>
      <td className="py-2 pr-2 text-gray-700">{formatWorldVersion(world.version)}</td>
      <td className="py-2 pr-2 text-gray-700">{formatBytes(world.sizeBytes)}</td>
      <td className="py-2 pr-2 text-gray-700">
        {formatDateTime(world.lastPlayed?.seconds) || '—'}
      </td>
      <td className="py-2">
        <div className="flex gap-1">
          {!world.active && (
            <Button disabled={disabled} onClick={() => onSwitch(world.name)}>
              切替
            </Button>
          )}
          <Button disabled={disabled} onClick={() => onClone(world.name)}>
            複製
          </Button>
          <Button disabled={disabled} onClick={() => onRename(world.name)}>
            改名
          </Button>
          {/* 稼働中のワールドは削除できない。先に切り替えてもらう。 */}
          <Button tone="danger" disabled={disabled || world.active} onClick={() => onDelete(world.name)}>
            削除
          </Button>
        </div>
      </td>
    </tr>
  )
}
