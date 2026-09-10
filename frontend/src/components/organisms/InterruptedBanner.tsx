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
      className="border-b border-warn-500 bg-warn-50 px-4 py-2"
      aria-label="中断された操作の警告"
    >
      <div className="mx-auto max-w-5xl text-sm text-warn-700">
        前回の実行が操作の途中で終了した形跡があります。ワールドの保存が止まったままの可能性があるため、
        <strong className="font-semibold">save-on を送信し直しました</strong>
        。念のためサーバーを再起動すると確実です。
      </div>
    </div>
  )
}
