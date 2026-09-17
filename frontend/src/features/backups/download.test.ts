import { afterEach, describe, expect, it, vi } from 'vitest'

import { startDownload } from './download'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('startDownload', () => {
  it('ファイル名を指定したリンクを開く', () => {
    let opened: { href: string; download: string; attached: boolean } | null = null
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      // 押された瞬間に DOM へ入っていること。外れていると Firefox が無視する。
      opened = {
        href: this.href,
        download: this.download,
        attached: document.body.contains(this),
      }
    })

    startDownload('/download/backup?t=abc', 'backup-26.2-world-20260916-0300.zip')

    expect(opened).toEqual({
      href: `${window.location.origin}/download/backup?t=abc`,
      download: 'backup-26.2-world-20260916-0300.zip',
      attached: true,
    })
  })

  // 押すたびに要素が増えると、画面に見えないリンクが積もる。
  it('後片付けしてリンクを残さない', () => {
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    startDownload('/download/backup?t=abc', 'backup.zip')

    expect(document.querySelectorAll('a')).toHaveLength(0)
  })

  // デモはバックエンドを持たないので data: の URL を渡す。
  it('data: の URL も開ける', () => {
    let href = ''
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      href = this.href
    })

    startDownload('data:text/plain;charset=utf-8,%E3%83%87%E3%83%A2', 'demo.txt')

    expect(href).toBe('data:text/plain;charset=utf-8,%E3%83%87%E3%83%A2')
  })
})
