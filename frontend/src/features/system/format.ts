/**
 * 使用率の帯に付ける色の段。
 *
 * 4GB の Pi で MC が実 RSS 約 2.7GB を使う前提なので、メモリは常に
 * それなりに埋まっている。80% を警告にすると常時警告になって意味を
 * 失うため、危険側に寄せて 90% から警告にする。
 */
export type UsageTone = 'ok' | 'warn' | 'danger'

export function usageTone(percent: number): UsageTone {
  if (percent >= 95) {
    return 'danger'
  }
  if (percent >= 90) {
    return 'warn'
  }
  return 'ok'
}

/** 使用率を「42.5%」の形にする。 */
export function percentLabel(percent: number): string {
  if (!Number.isFinite(percent)) {
    return '—'
  }
  return `${percent.toFixed(1)}%`
}

/**
 * CPU 使用率の測定区間を添える文言。
 *
 * どの期間の平均かを隠さない。1 点の値ではなく差分であることが
 * 読み手に伝わらないと、瞬間値だと思って判断されてしまう。
 */
export function windowLabel(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return ''
  }
  if (seconds < 60) {
    return `直近 ${Math.round(seconds)} 秒の平均`
  }
  return `直近 ${Math.round(seconds / 60)} 分の平均`
}

/** ロードアベレージ。コア数を添えないと大きいのか小さいのか判断できない。 */
export function loadLabel(load1: number, load5: number, load15: number, cores: number): string {
  const values = [load1, load5, load15].map((v) => v.toFixed(2)).join(' / ')
  return cores > 0 ? `${values}（${cores} コア）` : values
}
