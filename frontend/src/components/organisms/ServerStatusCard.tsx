import {
  containerStateLabel,
  playerCountLabel,
  savingStateLabel,
  serverTone,
  uptimeLabel,
  useServerStatus,
} from '../../features/server'
import type { ServerTone } from '../../features/server'
import { formatWorldVersion } from '../../features/worlds/worldVersion'
import { ContainerState } from '../../gen/mcadmin/v1/server_pb'
import type { GetStatusResponse } from '../../gen/mcadmin/v1/server_pb'
import { describeError } from '../../lib/errors'
import { Badge, PixelIcon, Spinner } from '../atoms'
import type { BadgeTone } from '../atoms'
import { PanelHeader, StatItem } from '../molecules'

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
        <div className="flex items-center gap-2.5 text-sm text-dim">
          <Spinner label="状態を取得しています" />
          <span>状態を取得しています…</span>
        </div>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <p className="text-sm text-danger-ink">{describeError(error)}</p>
      </Card>
    )
  }

  const tone = serverTone(data.containerState, data.healthy)

  return (
    <Card>
      <PanelHeader
        title="サーバーの状態"
        tag="SYS // STATUS"
        badge={
          <Badge tone={BADGE_TONES[tone]}>
            {containerStateLabel(data.containerState, data.healthy)}
          </Badge>
        }
      />
      <div className="grid gap-4 md:grid-cols-[auto_minmax(0,1fr)]">
        <Core tone={tone} word={coreWord(data)} />
        <dl className="stat-grid grid grid-cols-2 gap-3 sm:grid-cols-3">
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
      </div>
    </Card>
  )
}

const BADGE_TONES: Record<ServerTone, BadgeTone> = {
  ok: 'ok',
  warn: 'warn',
  off: 'neutral',
}

const CORE_WORDS: Record<ContainerState, string> = {
  [ContainerState.UNSPECIFIED]: 'UNKNOWN',
  [ContainerState.RUNNING]: 'ONLINE',
  [ContainerState.EXITED]: 'OFFLINE',
  [ContainerState.RESTARTING]: 'REBOOT',
  [ContainerState.MISSING]: 'NO UNIT',
}

/** 実行中でもヘルスチェック前は遊べない。バッジと同じく正常とは分けて見せる。 */
function coreWord(status: GetStatusResponse): string {
  if (status.containerState === ContainerState.RUNNING && !status.healthy) {
    return 'STARTING'
  }
  return CORE_WORDS[status.containerState] ?? 'UNKNOWN'
}

/**
 * 状態の色で光るブロック。
 *
 * 状態そのものはバッジが文字で伝えているので、ここは飾りとして
 * 読み上げから外す。狭い画面では場所を取るので出さない。
 */
function Core({ tone, word }: { tone: ServerTone; word: string }) {
  return (
    <div aria-hidden="true" className={`core core-${tone} hidden md:grid`}>
      <PixelIcon name="block" className="size-20" />
      <span className="core-label">{word}</span>
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
  return <section className="hud-panel flex flex-col gap-5">{children}</section>
}
