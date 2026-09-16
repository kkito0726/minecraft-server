/**
 * 受け取った URL を開いて、手元の PC に保存させる。
 *
 * `<a download>` を使う。リンクを辿るだけなので Authorization ヘッダーは
 * 付けられないが、URL に短命の受取券が入っているので認可はそれで足りる。
 * fetch で受け取って Blob にする方法もあるが、200MB のアーカイブが丸ごと
 * ブラウザのメモリに載り、中断した転送を再開できない。
 */
export function startDownload(url: string, fileName: string): void {
  const anchor = document.createElement('a')
  anchor.href = url
  // 同一オリジンなので download 属性のファイル名が効く。
  anchor.download = fileName
  anchor.rel = 'noopener'

  document.body.append(anchor)
  anchor.click()
  anchor.remove()
}
