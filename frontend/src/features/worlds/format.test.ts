import { describe, expect, it } from 'vitest'

import { formatBytes, formatDateTime } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1024, '1.0 KB'],
    [26_420_788, '25.2 MB'],
    [3_221_225_472, '3.0 GB'],
  ])('%s → %s', (bytes, want) => {
    expect(formatBytes(bytes)).toBe(want)
  })

  it('bigint も受け取る', () => {
    expect(formatBytes(26_420_788n)).toBe('25.2 MB')
  })

  // サイズを取れないことがある。NaN や -1 を出さない。
  it.each([-1, Number.NaN])('%s は 0 B', (bytes) => {
    expect(formatBytes(bytes)).toBe('0 B')
  })
})

describe('formatDateTime', () => {
  it('秒から日時にする', () => {
    const seconds = BigInt(Math.floor(new Date(2026, 8, 10, 16, 30).getTime() / 1000))
    expect(formatDateTime(seconds)).toBe('2026-09-10 16:30')
  })

  // level.dat を読めないと LastPlayed が無い。空欄にする。
  it('無ければ空', () => {
    expect(formatDateTime(undefined)).toBe('')
  })
})
