import { createClient } from '@connectrpc/connect'

import { BackupService } from '../../gen/mcadmin/v1/backup_pb'
import { transport } from '../../lib/transport'

/** BackupService のクライアント。認証 interceptor つきの transport を共有する。 */
export const backupClient = createClient(BackupService, transport)
