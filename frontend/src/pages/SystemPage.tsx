import { SystemMetricsCard } from '../components/organisms'

/**
 * ホスト（Pi 本体）の資源の使用状況。
 *
 * 開いている間だけ 5 秒ごとに取り直す。他の画面と違って、値が操作では
 * なく時間で変わるため。
 */
export function SystemPage() {
  return (
    <div className="flex flex-col gap-5">
      <SystemMetricsCard />
    </div>
  )
}
