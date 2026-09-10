import { BackupMode, RestoreTarget, VersionVerdict } from '../../gen/mcadmin/v1/backup_pb'
import type { BadgeTone } from '../../components/atoms'

/**
 * proto の列挙を画面の日本語にする。
 *
 * 生成コードの列挙は画面にそのまま出せない。ここに集めておくと
 * 「同じ列挙が画面ごとに違う言葉で出る」ことを防げる。
 */

export function modeLabel(mode: BackupMode): string {
  switch (mode) {
    case BackupMode.COLD:
      return '停止してから取る'
    case BackupMode.HOT:
    case BackupMode.UNSPECIFIED:
      return '稼働したまま取る'
  }
}

export function verdictLabel(verdict: VersionVerdict): string {
  switch (verdict) {
    case VersionVerdict.MATCH:
      return 'バージョンが一致しています'
    case VersionVerdict.OLDER_WILL_UPGRADE:
      return 'アーカイブの方が古いバージョンです'
    case VersionVerdict.NEWER_INCOMPATIBLE:
      return 'アーカイブの方が新しいバージョンです'
    case VersionVerdict.UNKNOWN:
    case VersionVerdict.UNSPECIFIED:
      return 'バージョンが不明です'
  }
}

/** 一致だけが安全。それ以外は必ず承諾を求める側に倒す。 */
export function verdictTone(verdict: VersionVerdict): BadgeTone {
  switch (verdict) {
    case VersionVerdict.MATCH:
      return 'ok'
    case VersionVerdict.NEWER_INCOMPATIBLE:
      return 'danger'
    default:
      return 'warn'
  }
}

export function targetLabel(target: RestoreTarget): string {
  switch (target) {
    case RestoreTarget.CURRENT_LEVEL:
      return '稼働中のワールド名で復元する'
    case RestoreTarget.ARCHIVE_LEVEL:
    case RestoreTarget.UNSPECIFIED:
      return 'アーカイブのワールド名で復元する'
  }
}
