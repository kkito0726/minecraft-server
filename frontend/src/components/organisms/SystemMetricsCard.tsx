import {
  loadLabel,
  percentLabel,
  usageTone,
  useSystemMetrics,
  windowLabel,
} from '../../features/system'
import type { UsageTone } from '../../features/system'
import type {
  CpuUsage,
  MemoryUsage,
  StorageUsage,
} from '../../gen/mcadmin/v1/system_pb'
import { describeError } from '../../lib/errors'
import { formatBytes } from '../../lib/format'
import { Spinner } from '../atoms'
import { PanelHeader, StatItem } from '../molecules'

/**
 * ホスト（Pi 本体）の CPU・メモリ・ストレージ。
 *
 * 項目ごとに「読めなかった」を持てる形にしてある。資源が苦しいときに
 * こそ開く画面なので、1 つ読めないだけで全体を空にしない。
 */
export function SystemMetricsCard() {
  const { data, isPending, error } = useSystemMetrics()

  if (isPending) {
    return (
      <Card>
        <div className="flex items-center gap-2.5 text-sm text-dim">
          <Spinner label="使用状況を取得しています" />
          <span>使用状況を取得しています…</span>
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

  return (
    <Card>
      <PanelHeader title="ホストの資源" tag="SYS // USAGE" />
      <div className="flex flex-col gap-5">
        <CpuMeter cpu={data.cpu} />
        <MemoryMeter memory={data.memory} />
        <StorageMeter storage={data.storage} />
      </div>
    </Card>
  )
}

function CpuMeter({ cpu }: { cpu: CpuUsage | undefined }) {
  if (!cpu) {
    return <Unavailable title="CPU" reason="値が返りませんでした" />
  }

  const detail = [windowLabel(cpu.windowSeconds), `${cpu.cores} コア`]
    .filter(Boolean)
    .join(' · ')

  return (
    <section className="flex flex-col gap-2.5">
      <Meter
        title="CPU"
        percent={cpu.usedPercent}
        available={cpu.available}
        reason={cpu.unavailableReason}
        detail={detail}
      />
      <dl className="stat-grid grid grid-cols-1 gap-3 sm:grid-cols-2">
        <StatItem label="ロードアベレージ">
          {loadLabel(cpu.load1, cpu.load5, cpu.load15, cpu.cores)}
        </StatItem>
        <StatItem label="測定区間">{windowLabel(cpu.windowSeconds) || '—'}</StatItem>
      </dl>
    </section>
  )
}

function MemoryMeter({ memory }: { memory: MemoryUsage | undefined }) {
  if (!memory) {
    return <Unavailable title="メモリ" reason="値が返りませんでした" />
  }

  return (
    <section className="flex flex-col gap-2.5">
      <Meter
        title="メモリ"
        percent={memory.usedPercent}
        available={memory.available}
        reason={memory.unavailableReason}
        detail={
          memory.available
            ? `${formatBytes(memory.usedBytes)} / ${formatBytes(memory.totalBytes)}`
            : ''
        }
      />
      {memory.available && (
        <dl className="stat-grid grid grid-cols-1 gap-3 sm:grid-cols-2">
          <StatItem label="使用できる量">{formatBytes(memory.availableBytes)}</StatItem>
          <StatItem label="スワップ">{swapLabel(memory)}</StatItem>
        </dl>
      )}
    </section>
  )
}

function StorageMeter({ storage }: { storage: StorageUsage | undefined }) {
  if (!storage) {
    return <Unavailable title="ストレージ" reason="値が返りませんでした" />
  }

  return (
    <section className="flex flex-col gap-2.5">
      <Meter
        title="ストレージ"
        percent={storage.usedPercent}
        available={storage.available}
        reason={storage.unavailableReason}
        detail={
          storage.available
            ? `${formatBytes(storage.usedBytes)} / ${formatBytes(storage.totalBytes)}`
            : ''
        }
      />
      {storage.available && (
        <dl className="stat-grid grid grid-cols-1 gap-3 sm:grid-cols-2">
          <StatItem label="空き">{formatBytes(storage.availableBytes)}</StatItem>
          <StatItem label="測定した場所">{storage.path || '—'}</StatItem>
        </dl>
      )}
    </section>
  )
}

/**
 * スワップは Pi では無効にしている前提。
 * 有効になっていること自体が気づきたい事実なので、0 のときも黙らせない。
 */
function swapLabel(memory: MemoryUsage): string {
  if (memory.swapTotalBytes <= 0n) {
    return '無効'
  }
  return `${formatBytes(memory.swapUsedBytes)} / ${formatBytes(memory.swapTotalBytes)}`
}

type MeterProps = {
  title: string
  percent: number
  available: boolean
  reason: string
  detail: string
}

function Meter({ title, percent, available, reason, detail }: MeterProps) {
  if (!available) {
    return <Unavailable title={title} reason={reason} />
  }

  const tone = usageTone(percent)

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[11px] font-medium tracking-wide text-faint">{title}</span>
        <span className="font-pixel text-lg text-fg">{percentLabel(percent)}</span>
      </div>
      <div
        role="meter"
        aria-label={`${title} の使用率`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(percent)}
        className="xp-bar w-full"
      >
        <div className={`xp-fill usage-${tone}`} style={{ width: `${clamp(percent)}%` }} />
      </div>
      {detail && <p className="text-xs text-dim">{detail}</p>}
    </div>
  )
}

function Unavailable({ title, reason }: { title: string; reason: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[11px] font-medium tracking-wide text-faint">{title}</span>
      <p className="text-sm text-dim">{reason || '取得できませんでした'}</p>
    </div>
  )
}

function clamp(percent: number): number {
  if (!Number.isFinite(percent)) {
    return 0
  }
  return Math.min(100, Math.max(0, percent))
}

function Card({ children }: { children: React.ReactNode }) {
  return <section className="hud-panel flex flex-col gap-5">{children}</section>
}

export type { UsageTone }
