import { createConnectTransport } from '@connectrpc/connect-web'
import type { Interceptor } from '@connectrpc/connect'

import { readToken } from '../features/auth/token'

/**
 * Connect のエンドポイントの基点。
 *
 * mcadmind は /rpc 配下にハンドラを登録し、それ以外を SPA として返す。
 * ここを "/" にすると index.html が 200 で返り、クライアント側は
 * JSON の解析エラーになる。404 ではないので原因が分かりにくい。
 */
export const RPC_BASE_URL = '/rpc'

/**
 * 共有トークンを Authorization ヘッダに載せる。
 *
 * トークンは**リクエストのたびに読み直す**。モジュールの読み込み時に
 * 捕まえると、入力して保存した後もページを再読み込みするまで
 * 古い値を送り続ける。症状は「認証が壊れている」に見える。
 *
 * 呼び出し側が明示的に付けたヘッダは上書きしない。入力されたばかりの
 * トークンを確かめるとき、そのトークンはまだ保存されていない
 * （通ったものだけ保存する方針のため）。保存済みの値で上書きすると、
 * 検証のリクエストが常にトークンなしで飛ぶ。
 */
export const authInterceptor: Interceptor = (next) => async (req) => {
  if (!req.header.has('Authorization')) {
    const token = readToken()
    if (token) {
      req.header.set('Authorization', `Bearer ${token}`)
    }
  }
  return next(req)
}

/**
 * アプリ全体で 1 つだけ持つトランスポート。
 *
 * 1 つに揃えるのは、操作の進捗を購読する server-streaming も
 * 同じ認証 interceptor を通す必要があるため。別に作ると
 * ストリームだけ認証が漏れる。
 */
export const transport = createConnectTransport({
  baseUrl: RPC_BASE_URL,
  interceptors: [authInterceptor],
})
