import { useState } from 'react'

import { OperationState } from '../../gen/mcadmin/v1/common_pb'
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
        'border-b px-4 py-2',
        failed ? 'border-danger-500 bg-danger-50' : 'border-gray-200 bg-white',
      ].join(' ')}
      role="status"
      aria-live="polite"
      aria-label="操作の進捗"
    >
      <div className="mx-auto flex max-w-5xl flex-col gap-2">
        <div className="flex items-center gap-3">
          {!done && <Spinner decorative />}
          <span className="text-sm font-medium text-gray-900">{kindLabel(operation.kind)}</span>
          <Badge tone={failed ? 'danger' : done ? 'ok' : 'neutral'}>
            {stateLabel(operation.state)}
          </Badge>
          <span className="text-xs text-gray-600">
            {operation.stepIndex}/{operation.stepTotal} {operation.currentStep}
          </span>
          <div className="ml-auto flex gap-1">
            <Button onClick={() => setExpanded((v) => !v)} aria-expanded={expanded}>
              {expanded ? 'ログを隠す' : `ログ (${log.length})`}
            </Button>
            {done && <Button onClick={dismiss}>閉じる</Button>}
          </div>
        </div>

        <StepProgress
          index={operation.stepIndex}
          total={operation.stepTotal}
          bytesDone={operation.bytesDone}
          bytesTotal={operation.bytesTotal}
        />

        {failed && operation.errorMessage && (
          <p className="text-sm text-danger-700">{operation.errorMessage}</p>
        )}

        {expanded && <LogPanel lines={log} />}
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

function LogPanel({ lines }: { lines: LogLine[] }) {
  return (
    <ol className="max-h-48 overflow-y-auto rounded bg-gray-50 p-2 text-xs">
      {lines.map((line) => (
        <li key={String(line.seq)} className="flex gap-2 py-0.5">
          <span className="shrink-0 text-gray-400">{String(line.seq)}</span>
          <span className={toneClass(levelTone(line.level))}>{line.message}</span>
        </li>
      ))}
    </ol>
  )
}

function toneClass(tone: 'neutral' | 'warn' | 'danger'): string {
  switch (tone) {
    case 'danger':
      return 'text-danger-700'
    case 'warn':
      return 'text-warn-700'
    default:
      return 'text-gray-700'
  }
}
