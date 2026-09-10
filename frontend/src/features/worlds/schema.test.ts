import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import {
  RESERVED_WORLD_NAMES,
  WORLD_NAME_PATTERN,
  isValidWorldName,
  worldNameError,
} from './schema'

/**
 * NFR-304: 検証規則をフロントとバックで同一にする。
 *
 * 口約束では守られない。片方だけが緩むと、不正な名前が
 * ファイルシステムに到達する。実際の Go のソースを読んで確かめる。
 */
describe('バックエンドとの一致', () => {
  const goSource = readFileSync(
    join(import.meta.dirname, '../../../../backend/internal/domain/world/name.go'),
    'utf8',
  )

  it('正規表現がバックエンドと同一', () => {
    const match = goSource.match(/const NamePattern = `([^`]+)`/)
    expect(match?.[1]).toBe(WORLD_NAME_PATTERN)
  })

  it('予約語がバックエンドと同一', () => {
    const match = goSource.match(/var reservedNames = \[\]string\{([^}]+)\}/)
    const names = [...(match?.[1] ?? '').matchAll(/"([^"]+)"/g)].map((m) => m[1]).sort()
    expect(names).toEqual([...RESERVED_WORLD_NAMES].sort())
  })
})

describe('isValidWorldName', () => {
  it.each([
    ['1 文字', 'a', true],
    ['32 文字', 'a'.repeat(32), true],
    ['英数字と記号', 'my_world-2', true],
    ['数字始まり', '2nd', true],
    ['33 文字', 'a'.repeat(33), false],
    ['空', '', false],
    ['記号始まり', '_world', false],
    ['親ディレクトリ', '..', false],
    ['スラッシュを含む', 'a/b', false],
    ['日本語', 'ワールド', false],
    ['空白を含む', 'my world', false],
    ['退避名', 'world.broken-20260101-000000', false],
  ])('%s: %s → %s', (_name, value, want) => {
    expect(isValidWorldName(value)).toBe(want)
  })

  // EDGE-102: data/ の既存ディレクトリと衝突する名前は使えない。
  it.each(RESERVED_WORLD_NAMES)('予約語 %s は使えない', (name) => {
    expect(isValidWorldName(name)).toBe(false)
  })
})

describe('worldNameError', () => {
  it('未入力のうちは何も言わない', () => {
    expect(worldNameError('')).toBe('')
  })

  it('使える名前なら空', () => {
    expect(worldNameError('world')).toBe('')
  })

  it('形式が違えば理由を返す', () => {
    expect(worldNameError('ワールド')).toContain('英数字')
  })

  it('予約語なら理由を返す', () => {
    expect(worldNameError('plugins')).toContain('既存のディレクトリ')
  })
})
