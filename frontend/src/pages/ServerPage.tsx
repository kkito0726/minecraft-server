import { ServerControls, ServerStatusCard } from '../components/organisms'

/**
 * サーバーの状態と起動・停止・再起動（REQ-001 / REQ-002）。
 */
export function ServerPage() {
  return (
    <div className="flex flex-col gap-5">
      <ServerStatusCard />
      <ServerControls />
    </div>
  )
}
