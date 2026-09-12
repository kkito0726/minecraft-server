/**
 * 進捗の帯。経験値バーのように 10 等分の刻みを入れる。
 *
 * 総量が不明な場合（bytes が取れないアーカイブなど）は
 * 不確定として表示する。
 */
export type ProgressBarProps = {
  done: number
  total: number
  label: string
}

export function ProgressBar({ done, total, label }: ProgressBarProps) {
  const indeterminate = total <= 0
  const ratio = indeterminate ? 0 : Math.min(1, Math.max(0, done / total))

  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={indeterminate ? undefined : total}
      aria-valuenow={indeterminate ? undefined : done}
      className="xp-bar w-full"
    >
      <div
        className={indeterminate ? 'xp-fill xp-fill-indeterminate' : 'xp-fill'}
        style={indeterminate ? undefined : { width: `${ratio * 100}%` }}
      />
    </div>
  )
}
