import { useState } from 'react'

import { OperationState } from '../../gen/mcadmin/v1/common_pb'
import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import { isTerminal, kindLabel, levelTone, stateLabel, useOperation } from '../../features/operations'
import type { LogLine } from '../../features/operations'
import { Badge, Button, ProgressBar, Spinner } from '../atoms'

/**
 * 進行中の操作を画面上部に貼り付けて示す帯。
 *
 * 終わった操作も消さずに残す。失敗したときの理由を伝える場所が
 * ここしかないため、終端で自動的に消すと「何も言わずに操作が消えた」
 * ことになる。閉じるのは利用者の操作に任せる。
 */
export function OperationBanner() {
  const { operation, log, dismiss } = useOperation()
  const [expanded, setExpanded] = useState(false)

  if (!operation) {
    return null
  }

  const done = isTerminal(operation)
  const failed = operation.state === OperationState.FAILED

  return (
    <div
      className={[
        'op-banner px-4 py-3 lg:px-8',
        failed ? 'op-banner-failed' : done ? 'op-banner-done' : 'op-banner-running',
      ].join(' ')}
      role="status"
      aria-live="polite"
      aria-label="操作の進捗"
    >
      <div className="mx-auto flex max-w-6xl flex-col gap-2.5">
        <BannerHeader
          operation={operation}
          done={done}
          failed={failed}
          logCount={log.length}
          expanded={expanded}
          onToggleLog={() => setExpanded((v) => !v)}
          onDismiss={dismiss}
        />

        <StepProgress
          index={operation.stepIndex}
          total={operation.stepTotal}
          bytesDone={operation.bytesDone}
          bytesTotal={operation.bytesTotal}
        />

        {failed && operation.errorMessage && (
          <p className="text-sm text-danger-ink">{operation.errorMessage}</p>
        )}

        {expanded && <LogPanel lines={log} />}
      </div>
    </div>
  )
}

type BannerHeaderProps = {
  operation: Operation
  done: boolean
  failed: boolean
  logCount: number
  expanded: boolean
  onToggleLog: () => void
  onDismiss: () => void
}

/** 何の操作が、いまどの手順にいるか。ログの開閉と、終わった帯を閉じる操作。 */
function BannerHeader({
  operation,
  done,
  failed,
  logCount,
  expanded,
  onToggleLog,
  onDismiss,
}: BannerHeaderProps) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      {!done && <Spinner decorative />}
      <span className="font-pixel text-sm text-fg">{kindLabel(operation.kind)}</span>
      <Badge tone={failed ? 'danger' : done ? 'ok' : 'neutral'}>
        {stateLabel(operation.state)}
      </Badge>
      <span className="text-xs text-dim tabular-nums">
        {operation.stepIndex}/{operation.stepTotal} {operation.currentStep}
      </span>
      <div className="ml-auto flex gap-1.5">
        <Button size="sm" onClick={onToggleLog} aria-expanded={expanded}>
          {expanded ? 'ログを隠す' : `ログ (${logCount})`}
        </Button>
        {done && (
          <Button size="sm" onClick={onDismiss}>
            閉じる
          </Button>
        )}
      </div>
    </div>
  )
}

type StepProgressProps = {
  index: number
  total: number
  bytesDone: bigint
  bytesTotal: bigint
}

/**
 * 進み具合の帯。
 *
 * バイト数が取れているときはそちらを優先する。ステップ単位より
 * 細かく動くので、30MB の展開中でも止まって見えない。
 * int64 は bigint で届くので、ここで number に落とす。
 */
function StepProgress({ index, total, bytesDone, bytesTotal }: StepProgressProps) {
  if (bytesTotal > 0n) {
    return <ProgressBar done={Number(bytesDone)} total={Number(bytesTotal)} label="処理の進み具合" />
  }
  return <ProgressBar done={index} total={total} label="手順の進み具合" />
}

/** 端末の出力のように見せる。行頭の記号は CSS が描き、文字には含めない。 */
function LogPanel({ lines }: { lines: LogLine[] }) {
  return (
    <ol className="log-panel max-h-56 overflow-y-auto border border-line bg-void/90 p-3 font-mono text-xs leading-relaxed">
      {lines.map((line) => (
        <li key={String(line.seq)} className="flex gap-2 py-0.5">
          <span className="w-8 shrink-0 text-right text-faint tabular-nums">{String(line.seq)}</span>
          <span className={toneClass(levelTone(line.level))}>{line.message}</span>
        </li>
      ))}
    </ol>
  )
}

function toneClass(tone: 'neutral' | 'warn' | 'danger'): string {
  switch (tone) {
    case 'danger':
      return 'text-danger-ink'
    case 'warn':
      return 'text-warn-ink'
    default:
      return 'text-dim'
  }
}
