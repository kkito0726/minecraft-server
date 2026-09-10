import { createClient } from '@connectrpc/connect'

import { ServerService } from '../../gen/mcadmin/v1/server_pb'
import { transport } from '../../lib/transport'

/**
 * ServerService のクライアント。
 *
 * 1 つだけ作って使い回す。呼び出しのたびに作ってもよいが、
 * 認証 interceptor を持つ transport を共有していることを明示したい。
 */
export const serverClient = createClient(ServerService, transport)
