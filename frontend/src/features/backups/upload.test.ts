import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UPLOAD_URL, uploadBackup } from './upload'

/**
 * XMLHttpRequest の差し替え。
 *
 * jsdom の実装は送信側の進捗を出さないので、経路そのものを偽物にする。
 */
class FakeXHR {
  static last: FakeXHR | null = null

  status = 200
  responseText = '{}'
  responseType = ''
  readonly headers: Record<string, string> = {}
  readonly upload = { onprogress: null as ((e: ProgressEvent) => void) | null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  onabort: (() => void) | null = null
  method = ''
  url = ''
  sent: unknown = null
  aborted = false

  constructor() {
    FakeXHR.last = this
  }

  open(method: string, url: string) {
    this.method = method
    this.url = url
  }

  setRequestHeader(key: string, value: string) {
    this.headers[key] = value
  }

  send(body: unknown) {
    this.sent = body
  }

  abort() {
    this.aborted = true
    this.onabort?.()
  }

  /** サーバーからの応答を再現する。 */
  respond(status: number, body: string) {
    this.status = status
    this.responseText = body
    this.onload?.()
  }
}

beforeEach(() => {
  vi.stubGlobal('XMLHttpRequest', FakeXHR)
  window.localStorage.setItem('mcadmin.token', 't'.repeat(64))
})

afterEach(() => {
  vi.unstubAllGlobals()
  window.localStorage.clear()
  FakeXHR.last = null
})

const file = () => new File(['zip'], 'world.zip', { type: 'application/zip' })

describe('uploadBackup', () => {
  it('保存したトークンを添えて送る', async () => {
    const promise = uploadBackup(file())
    const xhr = FakeXHR.last!

    expect(xhr.method).toBe('POST')
    expect(xhr.url).toBe(UPLOAD_URL)
    expect(xhr.headers.Authorization).toBe(`Bearer ${'t'.repeat(64)}`)

    xhr.respond(200, JSON.stringify({ id: 'a.zip', level: 'world', rewrapped: false }))
    await expect(promise).resolves.toMatchObject({ id: 'a.zip', level: 'world' })
  })

  // 本体をそのまま送る。FormData に包むと本文が二重になる。
  it('ファイルをそのまま送る', async () => {
    const f = file()
    const promise = uploadBackup(f)
    const xhr = FakeXHR.last!

    expect(xhr.sent).toBe(f)
    xhr.respond(200, '{}')
    await promise
  })

  it('送信の進み具合を伝える', async () => {
    const onProgress = vi.fn()
    const promise = uploadBackup(file(), { onProgress })
    const xhr = FakeXHR.last!

    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 50, total: 200 } as ProgressEvent)
    expect(onProgress).toHaveBeenCalledWith(0.25)

    xhr.respond(200, '{}')
    await promise
  })

  // 合計が分からないときに 0 除算の値を渡さない。
  it('合計が分からなければ進捗を出さない', async () => {
    const onProgress = vi.fn()
    const promise = uploadBackup(file(), { onProgress })
    const xhr = FakeXHR.last!

    xhr.upload.onprogress?.({ lengthComputable: false, loaded: 50, total: 0 } as ProgressEvent)
    expect(onProgress).not.toHaveBeenCalled()

    xhr.respond(200, '{}')
    await promise
  })

  /**
   * サーバーは利用者が直せる理由を日本語で返す。
   * 「400 エラー」とだけ出すと、何をどう直せばよいか分からない。
   */
  it('サーバーが返した理由をそのまま伝える', async () => {
    const promise = uploadBackup(file())
    FakeXHR.last!.respond(
      400,
      JSON.stringify({ message: 'level.dat が zip の直下にあります。' }),
    )

    await expect(promise).rejects.toThrow('level.dat が zip の直下にあります。')
  })

  it('理由が無ければ状態コードを添える', async () => {
    const promise = uploadBackup(file())
    FakeXHR.last!.respond(500, 'not json')

    await expect(promise).rejects.toThrow('500')
  })

  it('接続できなければその旨を伝える', async () => {
    const promise = uploadBackup(file())
    FakeXHR.last!.onerror?.()

    await expect(promise).rejects.toThrow('接続できません')
  })

  it('中止できる', async () => {
    const controller = new AbortController()
    const promise = uploadBackup(file(), { signal: controller.signal })

    controller.abort()
    expect(FakeXHR.last!.aborted).toBe(true)
    await expect(promise).rejects.toThrow('中止')
  })
})
