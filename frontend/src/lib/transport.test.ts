import { afterEach, describe, expect, it } from 'vitest'

import { clearToken, writeToken } from '../features/auth/token'
import { RPC_BASE_URL, authInterceptor } from './transport'

afterEach(() => {
  window.localStorage.clear()
})

/** Connect のリクエストのうち、interceptor が触る部分だけを模したもの。 */
function fakeRequest() {
  return { header: new Headers() }
}

async function runInterceptor(): Promise<Headers> {
  const req = fakeRequest()
  // 型は Connect の内部形に依存するため、この試験の関心である header だけを渡す
  const next = async (r: typeof req) => r
  await (authInterceptor as unknown as (n: typeof next) => typeof next)(next)(req)
  return req.header
}

describe('authInterceptor', () => {
  it('保存されたトークンを Bearer として付ける', async () => {
    writeToken('a'.repeat(64))
    expect((await runInterceptor()).get('Authorization')).toBe(`Bearer ${'a'.repeat(64)}`)
  })

  it('トークンが無ければ何も付けない', async () => {
    clearToken()
    expect((await runInterceptor()).has('Authorization')).toBe(false)
  })

  /**
   * これがいちばん壊れやすい。モジュールの読み込み時にトークンを
   * 捕まえてしまうと、入力して保存した後もリロードするまで
   * 古い（空の）トークンを送り続ける。症状は「認証が壊れている」に見える。
   */
  it('リクエストのたびに読み直す', async () => {
    clearToken()
    expect((await runInterceptor()).has('Authorization')).toBe(false)

    writeToken('b'.repeat(64))
    expect((await runInterceptor()).get('Authorization')).toBe(`Bearer ${'b'.repeat(64)}`)

    writeToken('c'.repeat(64))
    expect((await runInterceptor()).get('Authorization')).toBe(`Bearer ${'c'.repeat(64)}`)
  })

  /**
   * 入力されたばかりのトークンはまだ保存されていない。通ったものだけを
   * 保存する方針なので、検証のときだけ呼び出し側がヘッダを付ける。
   * ここで上書きすると、検証のリクエストが常にトークンなしで飛ぶ。
   */
  it('呼び出し側が付けたヘッダは上書きしない', async () => {
    writeToken('a'.repeat(64))

    const req = fakeRequest()
    req.header.set('Authorization', 'Bearer explicit-token')
    const next = async (r: typeof req) => r
    await (authInterceptor as unknown as (n: typeof next) => typeof next)(next)(req)

    expect(req.header.get('Authorization')).toBe('Bearer explicit-token')
  })
})

describe('RPC_BASE_URL', () => {
  /**
   * Go 側は mux に rpcPrefix + パス で登録し、http.StripPrefix("/rpc") を通す。
   * ここを "/" にすると SPA の index.html が 200 で返り、
   * クライアントは JSON の解析エラーになる。404 ではないので原因が分かりにくい。
   */
  it('/rpc を指す', () => {
    expect(RPC_BASE_URL).toBe('/rpc')
  })
})
