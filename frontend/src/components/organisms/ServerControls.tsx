import { useState } from 'react'

import { useOperation } from '../../features/operations'
import { ACTION_LABELS, useServerActions, useServerStatus } from '../../features/server'
import type { ServerAction } from '../../features/server'
import { ContainerState } from '../../gen/mcadmin/v1/server_pb'
import { describeError } from '../../lib/errors'
import { Button } from '../atoms'

/**
 * サーバーの起動・停止・再起動。
 *
 * 実行中かどうかの判断は購読している操作にだけ従う（REQ-201）。
 * GetStatus も active_operation を持っているが、問い合わせの
 * キャッシュと購読の間隔がずれるため、両方を見るとボタンがちらつく。
 */
export function ServerControls() {
  const { isBusy } = useOperation()
  const { data } = useServerStatus()
  const actions = useServerActions()
  const [pendingConfirm, setPendingConfirm] = useState<ServerAction | null>(null)

  const running = data?.containerState === ContainerState.RUNNING
  const online = data?.onlinePlayers ?? 0

  const run = (action: ServerAction) => {
    // 人がいる状態での停止・再起動は接続を切る。一度確かめる。
    if (online > 0 && action !== 'start' && pendingConfirm !== action) {
      setPendingConfirm(action)
      return
    }
    setPendingConfirm(null)
    actions.mutate(action)
  }

  return (
    <section className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-gray-900">操作</h2>

      <div className="flex gap-2">
        <Button tone="primary" disabled={isBusy || running} onClick={() => run('start')}>
          {ACTION_LABELS.start}
        </Button>
        <Button disabled={isBusy || !running} onClick={() => run('restart')}>
          {ACTION_LABELS.restart}
        </Button>
        <Button tone="danger" disabled={isBusy || !running} onClick={() => run('stop')}>
          {ACTION_LABELS.stop}
        </Button>
      </div>

      {pendingConfirm && (
        <ConfirmNotice
          action={pendingConfirm}
          online={online}
          onConfirm={() => run(pendingConfirm)}
          onCancel={() => setPendingConfirm(null)}
        />
      )}

      {actions.error && <p className="text-sm text-danger-700">{describeError(actions.error)}</p>}

      {isBusy && <p className="text-xs text-gray-500">操作の実行中は変更できません。</p>}
    </section>
  )
}

type ConfirmNoticeProps = {
  action: ServerAction
  online: number
  onConfirm: () => void
  onCancel: () => void
}

function ConfirmNotice({ action, online, onConfirm, onCancel }: ConfirmNoticeProps) {
  return (
    <div className="flex flex-col gap-2 rounded border border-warn-500 bg-warn-50 p-3">
      <p className="text-sm text-warn-700">
        {online} 人が接続しています。{ACTION_LABELS[action]}すると全員が切断されます。
      </p>
      <div className="flex gap-2">
        <Button tone="danger" onClick={onConfirm}>
          {ACTION_LABELS[action]}する
        </Button>
        <Button onClick={onCancel}>やめる</Button>
      </div>
    </div>
  )
}
