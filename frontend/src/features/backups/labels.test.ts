import { describe, expect, it } from 'vitest'

import { BackupMode, RestoreTarget, VersionVerdict } from '../../gen/mcadmin/v1/backup_pb'
import { modeLabel, targetLabel, verdictLabel, verdictTone } from './labels'

describe('modeLabel', () => {
  it.each([
    [BackupMode.HOT, '稼働したまま'],
    [BackupMode.COLD, '停止してから'],
  ])('%s を説明する', (mode, expected) => {
    expect(modeLabel(mode)).toContain(expected)
  })
})

describe('verdictLabel', () => {
  it.each([
    [VersionVerdict.MATCH, '一致'],
    [VersionVerdict.OLDER_WILL_UPGRADE, '古い'],
    [VersionVerdict.NEWER_INCOMPATIBLE, '新しい'],
    [VersionVerdict.UNKNOWN, '不明'],
  ])('%s を日本語にする', (verdict, expected) => {
    expect(verdictLabel(verdict)).toContain(expected)
  })

  // 未知の値でも空にしない。空だと「判定が無い」ように見える。
  it('未指定でも文言を返す', () => {
    expect(verdictLabel(VersionVerdict.UNSPECIFIED)).not.toBe('')
  })
})

describe('verdictTone', () => {
  it('一致だけが安全な見た目になる', () => {
    expect(verdictTone(VersionVerdict.MATCH)).toBe('ok')
  })

  it.each([
    VersionVerdict.OLDER_WILL_UPGRADE,
    VersionVerdict.NEWER_INCOMPATIBLE,
    VersionVerdict.UNKNOWN,
  ])('%s は注意を促す', (verdict) => {
    expect(verdictTone(verdict)).not.toBe('ok')
  })
})

describe('targetLabel', () => {
  it.each([
    [RestoreTarget.ARCHIVE_LEVEL, 'アーカイブ'],
    [RestoreTarget.CURRENT_LEVEL, '稼働中'],
  ])('%s を説明する', (target, expected) => {
    expect(targetLabel(target)).toContain(expected)
  })
})
