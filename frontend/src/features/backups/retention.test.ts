import { describe, expect, it } from 'vitest'

import { parseRetention, retentionError } from './retention'

/**
 * 保持ポリシーの入力検証。
 *
 * keepCount が 0 だとバックアップが全損しうる。バックエンドの
 * 値オブジェクトが 1 以上を不変条件にしているので、画面でも同じ
 * 境界で止める（EDGE-103 / EDGE-104）。
 */
describe('parseRetention', () => {
  it('妥当な値を数値にする', () => {
    expect(parseRetention('10', '7')).toEqual({ ok: true, keepCount: 10, keepDays: 7 })
  })

  it('日数 0 は無効化を意味するので通す', () => {
    expect(parseRetention('1', '0')).toEqual({ ok: true, keepCount: 1, keepDays: 0 })
  })

  it.each([
    ['世代数 0', '0', '7'],
    ['世代数が負', '-1', '0'],
    ['世代数が空', '', '0'],
    ['世代数が小数', '1.5', '0'],
    ['世代数が数字でない', 'いくつか', '0'],
    ['日数が負', '10', '-1'],
    ['日数が空', '10', ''],
  ])('%s は拒否する', (_name, keepCount, keepDays) => {
    expect(parseRetention(keepCount, keepDays).ok).toBe(false)
  })
})

describe('retentionError', () => {
  it('妥当なら理由を返さない', () => {
    expect(retentionError('10', '7')).toBeUndefined()
  })

  it('世代数が 0 のとき最低 1 世代が要る理由を出す', () => {
    expect(retentionError('0', '7')).toContain('1')
  })

  it('日数が負のとき理由を出す', () => {
    expect(retentionError('10', '-1')).not.toBeUndefined()
  })
})
