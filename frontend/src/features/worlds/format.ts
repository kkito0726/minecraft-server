/** バイト数を読みやすくする。ワールドは 25MB 前後になる。 */
export function formatBytes(bytes: bigint | number): string {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024)))
  const scaled = value / 1024 ** exponent
  return `${exponent === 0 ? scaled : scaled.toFixed(1)} ${units[exponent]}`
}

/** 日時を「2026-09-10 16:30」の形にする。無ければ空。 */
export function formatDateTime(seconds: bigint | undefined): string {
  if (seconds === undefined) {
    return ''
  }
  const d = new Date(Number(seconds) * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
