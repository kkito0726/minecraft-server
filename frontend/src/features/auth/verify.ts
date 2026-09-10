import { createClient } from '@connectrpc/connect'

import { ServerService } from '../../gen/mcadmin/v1/server_pb'
import { transport } from '../../lib/transport'

/**
 * 渡されたトークンでサーバーに触れるかを確かめる。
 *
 * GetStatus を使うのは、副作用が無く、認証を通らないと必ず
 * Unauthenticated になるため。認証だけを確かめる専用の RPC は用意しない。
 *
 * トークンを**明示的にヘッダへ載せる**。入力されたばかりのトークンは
 * まだ保存されていない（通ったものだけ保存する方針のため）ので、
 * interceptor が localStorage から読む値に頼ると検証が必ず失敗する。
 */
export async function verifyToken(token: string): Promise<void> {
  const client = createClient(ServerService, transport)
  await client.getStatus({}, { headers: { Authorization: `Bearer ${token}` } })
}
