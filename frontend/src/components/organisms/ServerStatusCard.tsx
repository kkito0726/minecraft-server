import {
  containerStateLabel,
  playerCountLabel,
  savingStateLabel,
  uptimeLabel,
  useServerStatus,
} from '../../features/server'
import { formatWorldVersion } from '../../features/worlds/worldVersion'
import { ContainerState } from '../../gen/mcadmin/v1/server_pb'
import type { GetStatusResponse } from '../../gen/mcadmin/v1/server_pb'
import { describeError } from '../../lib/errors'
import { Badge, Spinner } from '../atoms'
import { StatItem } from '../molecules'

/**
 * サーバーの現在の状態。
 *
 * README が「STATUS が Up ... (healthy) かどうかで判断する」と書いている
 * とおり、「実行中であること」と「遊べる状態であること」は別の事実として示す。
 */
export function ServerStatusCard() {
  const { data, isPending, error } = useServerStatus()

  if (isPending) {
    return (
      <Card>
        <div className="flex items-center gap-2 text-sm text-gray-600">
          <Spinner label="状態を取得しています" />
          <span>状態を取得しています…</span>
        </div>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <p className="text-sm text-danger-700">{describeError(error)}</p>
      </Card>
    )
  }

  return (
    <Card>
      <StatusHeader status={data} />
      <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <StatItem label="稼働中のワールド">{data.activeLevel || '未設定'}</StatItem>
        <StatItem label="設定バージョン">{data.configuredVersion || '不明'}</StatItem>
        <StatItem label="ワールドのバージョン">
          {formatWorldVersion(data.activeWorldVersion)}
        </StatItem>
        <StatItem label="オンライン人数">
          {playerCountLabel(data.onlinePlayers, data.maxPlayers)}
        </StatItem>
        <StatItem label="稼働時間">{uptime(data) || '—'}</StatItem>
        <StatItem label="ワールドの保存">{savingStateLabel(data.savingState)}</StatItem>
      </dl>
    </Card>
  )
}

function StatusHeader({ status }: { status: GetStatusResponse }) {
  const running = status.containerState === ContainerState.RUNNING

  return (
    <div className="flex items-center gap-2">
      <h2 className="text-sm font-semibold text-gray-900">サーバーの状態</h2>
      <Badge tone={running && status.healthy ? 'ok' : running ? 'warn' : 'neutral'}>
        {containerStateLabel(status.containerState, status.healthy)}
      </Badge>
    </div>
  )
}

function uptime(status: GetStatusResponse): string {
  if (!status.containerStartedAt) {
    return ''
  }
  const startedAt = new Date(Number(status.containerStartedAt.seconds) * 1000)
  return uptimeLabel(startedAt, new Date())
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-4 rounded border border-gray-200 bg-white p-4">
      {children}
    </section>
  )
}
