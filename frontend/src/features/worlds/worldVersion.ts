import type { WorldVersion } from '../../gen/mcadmin/v1/common_pb.js'

/**
 * ワールドのバージョンを画面表示用の文字列にする。
 *
 * level.dat を読めなかった場合に空文字を返すと「バージョンが空のワールド」に
 * 見えてしまうため、明示的に「不明」と表示する（EDGE-001）。
 */
export function formatWorldVersion(v: WorldVersion | undefined): string {
  if (!v?.readable) {
    return '不明'
  }
  const suffix = v.snapshot ? '（スナップショット）' : ''
  return `${v.name}${suffix}`
}

/**
 * 2 つのワールドのバージョンを比較する。
 *
 * 比較に使うのは表示文字列 name ではなく整数の dataVersion（REQ-412）。
 * 片方でも読めない場合は 'unknown' を返し、呼び出し側に承諾を求めさせる。
 */
export type VersionComparison = 'match' | 'older' | 'newer' | 'unknown'

export function compareWorldVersions(
  archive: WorldVersion | undefined,
  current: WorldVersion | undefined,
): VersionComparison {
  if (!archive?.readable || !current?.readable) {
    return 'unknown'
  }
  if (archive.dataVersion === current.dataVersion) {
    return 'match'
  }
  return archive.dataVersion < current.dataVersion ? 'older' : 'newer'
}
