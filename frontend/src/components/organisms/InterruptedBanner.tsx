import { useServerStatus } from '../../features/server'

/**
 * 前回の実行が操作の途中で終わっていたことを知らせる（REQ-203）。
 *
 * バックエンドは起動時に無条件で save-on を送り直すが、RCON には
 * 「保存が有効か」を問い合わせる手段がないため、それが効いたかどうかは
 * 確かめられない。save-off が残ったままだと以降の変更がディスクに
 * 書かれないのに症状が何も出ないので、断定せずに事実だけを伝え続ける。
 *
 * どの画面にいても関係するため、帯として出す。
 */
export function InterruptedBanner() {
  const { data } = useServerStatus()

  if (!data?.interruptedOperationDetected) {
    return null
  }

  return (
    <div
      role="alert"
      className="hazard-edge border-b border-warn/50 bg-warn-soft/95 py-2.5 pr-4 pl-6 backdrop-blur lg:pr-8 lg:pl-10"
      aria-label="中断された操作の警告"
    >
      <div className="mx-auto max-w-6xl text-sm text-warn-ink">
        前回の実行が操作の途中で終了した形跡があります。ワールドの保存が止まったままの可能性があるため、
        <strong className="font-semibold text-warn">save-on を送信し直しました</strong>
        。念のためサーバーを再起動すると確実です。
      </div>
    </div>
  )
}
