import { createClient } from '@connectrpc/connect'

import { SystemService } from '../../gen/mcadmin/v1/system_pb'
import { transport } from '../../lib/transport'

/** SystemService のクライアント。serverClient と同じく 1 つを使い回す。 */
export const systemClient = createClient(SystemService, transport)
