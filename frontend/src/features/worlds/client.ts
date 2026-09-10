import { createClient } from '@connectrpc/connect'

import { WorldService } from '../../gen/mcadmin/v1/world_pb'
import { transport } from '../../lib/transport'

/** WorldService のクライアント。認証 interceptor つきの transport を共有する。 */
export const worldClient = createClient(WorldService, transport)
