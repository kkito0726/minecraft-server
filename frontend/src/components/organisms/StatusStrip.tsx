import {
  containerStateLabel,
  playerCountLabel,
  serverTone,
  useServerStatus,
} from '../../features/server'
import type { ServerTone } from '../../features/server'
import type { GetStatusResponse } from '../../gen/mcadmin/v1/server_pb'

/**
 * 画面上端の概況。どの画面にいても、サーバーがいまどうなっているかを示す。
 *
 * 要約だけを出し、操作は置かない。詳しい状態はサーバー画面のカードが持つ。
 * 見出しにはしない。サーバー画面の「サーバーの状態」と同じ役割の
 * 見出しが 2 つ並ぶと、見出しで画面を渡り歩く人が迷う。
 */
export function StatusStrip() {
  const { data, error } = useServerStatus()

  return (
    <div className="status-strip">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-1.5 px-4 py-2.5 text-xs lg:px-8">
        <Summary status={data} failed={Boolean(error)} />
        <span aria-hidden="true" className="hud-tag ml-auto hidden sm:inline">
          MCADMIN // CONSOLE
        </span>
      </div>
    </div>
  )
}

function Summary({ status, failed }: { status: GetStatusResponse | undefined; failed: boolean }) {
  if (failed) {
    return <Indicator tone="danger" label="状態を取得できません" />
  }
  if (!status) {
    return <Indicator tone="off" label="状態を確認しています…" />
  }

  return (
    <>
      <Indicator
        tone={serverTone(status.containerState, status.healthy)}
        label={containerStateLabel(status.containerState, status.healthy)}
      />
      <Readout label="WORLD" value={status.activeLevel || '未設定'} />
      <Readout label="PLAYERS" value={playerCountLabel(status.onlinePlayers, status.maxPlayers)} />
      <Readout label="VERSION" value={status.configuredVersion || '不明'} />
    </>
  )
}

function Indicator({ tone, label }: { tone: ServerTone | 'danger'; label: string }) {
  return (
    <span className="flex items-center gap-2.5 font-semibold text-fg">
      <span aria-hidden="true" className={`led led-${tone}`} />
      {label}
    </span>
  )
}

function Readout({ label, value }: { label: string; value: string }) {
  return (
    <span className="flex items-baseline gap-2">
      <span className="hud-tag">{label}</span>
      <span className="font-pixel text-sm text-dim">{value}</span>
    </span>
  )
}
