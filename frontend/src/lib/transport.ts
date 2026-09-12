import { createConnectTransport } from '@connectrpc/connect-web'
import type { Interceptor, Transport } from '@connectrpc/connect'

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

/** 実際のサーバーに繋ぐ口。 */
let active: Transport = createConnectTransport({
  baseUrl: RPC_BASE_URL,
  interceptors: [authInterceptor],
})

/**
 * 通信の口を差し替える。公開デモがブラウザ内の実装に繋ぎ替えるために使う。
 *
 * 差し替えても下の transport の同一性は変わらない。各 client は
 * 読み込み時に transport を捕まえるので、ここで参照ごと入れ替えると
 * 読み込みの順序に依存する壊れ方をする。
 */
export function setTransport(next: Transport): void {
  active = next
}

/**
 * アプリ全体で 1 つだけ持つトランスポート。
 *
 * 1 つに揃えるのは、操作の進捗を購読する server-streaming も
 * 同じ認証 interceptor を通す必要があるため。別に作ると
 * ストリームだけ認証が漏れる。
 *
 * 実体を直接見せず、いま有効な口へ渡すだけの薄い包みにしてある。
 */
export const transport: Transport = {
  unary: (...args) => active.unary(...args),
  stream: (...args) => active.stream(...args),
}
