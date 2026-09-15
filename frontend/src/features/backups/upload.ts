import { readToken } from '../auth/token'

/**
 * zip のアップロード先。
 *
 * Connect ではなく素の POST にしてあるのは、ブラウザが平文の HTTP/2 へ
 * 昇格せず client-streaming の RPC を使えないため。単項 RPC では本体を
 * 丸ごとメモリに載せることになり、4GB の Pi では選べない。
 */
export const UPLOAD_URL = '/upload/backup'

/** 取り込みの結果。サーバーが返す JSON と同じ形。 */
export type UploadResult = {
  id: string
  level: string
  /** アーカイブ内の level.dat から読んだ版。読めなければ空。 */
  version: string
  sizeBytes: number
  /** data/ 配下へ包み直したか。 */
  rewrapped: boolean
}

export type UploadOptions = {
  /** 送信の進み具合（0〜1）。合計が分からない場合は呼ばれない。 */
  onProgress?: (ratio: number) => void
  signal?: AbortSignal
}

/**
 * zip を送って取り込む。
 *
 * fetch ではなく XMLHttpRequest を使うのは、送信側の進捗が取れるのが
 * 今のところこちらだけのため。30MB のワールドを無反応で待たせない。
 */
export async function uploadBackup(
  file: File,
  options: UploadOptions = {},
): Promise<UploadResult> {
  // zip の受け口だけは Connect を通らないので、トランスポートの
  // 差し替えでは代われない。デモではここで偽の取り込みに寄せる。
  if (__DEMO__) {
    const { uploadDemoBackup } = await import('../../demo/upload')
    return uploadDemoBackup(file, options)
  }

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', UPLOAD_URL)
    xhr.responseType = 'text'

    const token = readToken()
    if (token) {
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    }
    xhr.setRequestHeader('Content-Type', 'application/zip')

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && options.onProgress) {
        options.onProgress(e.loaded / e.total)
      }
    }
    xhr.onload = () => finish(xhr, resolve, reject)
    xhr.onerror = () => reject(new Error('サーバーに接続できませんでした。'))
    xhr.onabort = () => reject(new Error('アップロードを中止しました。'))

    options.signal?.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(file)
  })
}

function finish(
  xhr: XMLHttpRequest,
  resolve: (value: UploadResult) => void,
  reject: (reason: Error) => void,
) {
  const body = parseBody(xhr.responseText)

  if (xhr.status >= 200 && xhr.status < 300) {
    resolve(body as UploadResult)
    return
  }
  // サーバーは利用者が直せる理由を日本語で返す。あるならそれを見せる。
  const message = typeof body?.message === 'string' ? body.message : ''
  reject(new Error(message || `取り込みに失敗しました（${xhr.status}）。`))
}

function parseBody(text: string): Record<string, unknown> | null {
  try {
    return JSON.parse(text) as Record<string, unknown>
  } catch {
    return null
  }
}
