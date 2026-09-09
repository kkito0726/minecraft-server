import { describe, expect, it } from 'vitest'

import { create } from '@bufbuild/protobuf'

import { WorldVersionSchema } from '../../gen/mcadmin/v1/common_pb.js'
import { compareWorldVersions, formatWorldVersion } from './worldVersion.js'

// 実測値: data/world/level.dat は name="26.2" / dataVersion=4903
const v = (dataVersion: number, opts: Partial<{ readable: boolean; name: string; snapshot: boolean }> = {}) =>
  create(WorldVersionSchema, {
    readable: opts.readable ?? true,
    name: opts.name ?? '26.2',
    dataVersion,
    snapshot: opts.snapshot ?? false,
    levelName: 'world',
  })

describe('formatWorldVersion', () => {
  it('読めたバージョンをそのまま表示する', () => {
    expect(formatWorldVersion(v(4903))).toBe('26.2')
  })

  it('スナップショットであることを併記する', () => {
    expect(formatWorldVersion(v(4903, { snapshot: true }))).toBe('26.2（スナップショット）')
  })

  it('読めなかった場合は「不明」を返す', () => {
    expect(formatWorldVersion(v(0, { readable: false }))).toBe('不明')
  })

  it('未定義の場合も「不明」を返す', () => {
    expect(formatWorldVersion(undefined)).toBe('不明')
  })
})

describe('compareWorldVersions', () => {
  it.each([
    ['一致', 4903, 4903, 'match'],
    ['アーカイブの方が古い', 4820, 4903, 'older'],
    ['アーカイブの方が新しい', 4950, 4903, 'newer'],
  ] as const)('%s', (_name, archive, current, expected) => {
    expect(compareWorldVersions(v(archive), v(current))).toBe(expected)
  })

  it('表示文字列が同じでも dataVersion が違えば一致としない', () => {
    const archive = v(4820, { name: '26.2' })
    const current = v(4903, { name: '26.2' })
    expect(compareWorldVersions(archive, current)).toBe('older')
  })

  it.each([
    ['アーカイブが読めない', v(0, { readable: false }), v(4903)],
    ['現在のワールドが読めない', v(4903), v(0, { readable: false })],
    ['どちらも未定義', undefined, undefined],
  ] as const)('%s 場合は unknown', (_name, archive, current) => {
    expect(compareWorldVersions(archive, current)).toBe('unknown')
  })
})
