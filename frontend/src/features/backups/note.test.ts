import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { MAX_NOTE_LENGTH, noteSlug } from './note'

/**
 * メモはファイル名の一部になる。バックエンドの NoteSlug が
 * 使える文字だけを残すため、日本語のメモは丸ごと消える。
 *
 * 消えること自体は仕様だが、押したあとに気づくのでは遅い。
 * 画面で結果を先に見せるために、同じ規則をこちらでも持つ。
 */
describe('noteSlug', () => {
  it.each([
    ['英数字はそのまま残る', 'before-update', 'before-update'],
    ['下線も残る', 'pre_fix', 'pre_fix'],
    ['空白はハイフンになる', 'before update', 'before-update'],
    ['連続した記号はまとめられる', 'a  ///  b', 'a-b'],
    ['前後の記号は落ちる', '--abc--', 'abc'],
    ['日本語は丸ごと消える', 'アップデート前', ''],
    ['日本語と英数字が混ざると英数字だけ残る', '更新前 v2', 'v2'],
    ['空なら空', '', ''],
  ])('%s', (_name, input, expected) => {
    expect(noteSlug(input)).toBe(expected)
  })

  it('上限で切り詰める', () => {
    expect(noteSlug('a'.repeat(MAX_NOTE_LENGTH + 10))).toHaveLength(MAX_NOTE_LENGTH)
  })

  // NFR-304。規則の出典はバックエンドにあり、こちらは写しである。
  // 片方だけ変わると、画面の予告と実際のファイル名がずれる。
  it('上限がバックエンドと一致している', () => {
    const source = readFileSync(
      join(import.meta.dirname, '../../../../backend/internal/domain/backup/naming.go'),
      'utf8',
    )
    expect(source).toContain(`maxNoteLength = ${MAX_NOTE_LENGTH}`)
  })
})
