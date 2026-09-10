import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

/**
 * Atomic Design の層の規約を機械的に検証する。
 *
 * 規約は口で言うだけでは守られない。「ここで proto の型を使えば早い」が
 * 一度通ると以降なし崩しになり、コンパイルは通ってしまう。
 */

const COMPONENTS_DIR = join(import.meta.dirname, '..')

function sourceFilesIn(layer: string): { path: string; source: string }[] {
  const dir = join(COMPONENTS_DIR, layer)
  let entries: string[]
  try {
    entries = readdirSync(dir)
  } catch {
    return []
  }

  return entries
    .filter((name) => /\.tsx?$/.test(name) && !name.endsWith('.test.tsx'))
    .filter((name) => statSync(join(dir, name)).isFile())
    .map((name) => ({
      path: `${layer}/${name}`,
      source: readFileSync(join(dir, name), 'utf8'),
    }))
}

function importsOf(source: string): string[] {
  const pattern = /(?:from|import)\s+['"]([^'"]+)['"]/g
  return [...source.matchAll(pattern)].map((m) => m[1]!)
}

describe('atoms', () => {
  const files = sourceFilesIn('atoms')

  it('コンポーネントが存在する', () => {
    expect(files.length).toBeGreaterThan(0)
  })

  // atoms が proto の型を知ると、生成コードの変更がデザイン部品に波及する。
  it.each(files)('$path は生成された proto の型を import しない', ({ source }) => {
    const offending = importsOf(source).filter((i) => i.includes('/gen/') || i.includes('_pb'))
    expect(offending).toEqual([])
  })

  // atoms が通信を知ると、見た目だけを差し替えることができなくなる。
  it.each(files)('$path は通信層を import しない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) =>
        i.includes('@connectrpc') ||
        i.includes('@tanstack/react-query') ||
        i.includes('/lib/transport'),
    )
    expect(offending).toEqual([])
  })

  // atoms が上位層を import すると循環になる。
  it.each(files)('$path は上位の層を import しない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) =>
        i.includes('molecules') ||
        i.includes('organisms') ||
        i.includes('templates') ||
        i.includes('/pages/') ||
        i.includes('/features/'),
    )
    expect(offending).toEqual([])
  })
})

describe('molecules', () => {
  const files = sourceFilesIn('molecules')

  it.each(files)('$path は organisms 以上を import しない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) => i.includes('organisms') || i.includes('templates') || i.includes('/pages/'),
    )
    expect(offending).toEqual([])
  })

  // molecules が通信を知ると、organisms との境界が消える。
  // 「業務上の意味を持ち、フックを使う」のは organisms の役割。
  it.each(files)('$path は通信層を import しない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) =>
        i.includes('@connectrpc') ||
        i.includes('@tanstack/react-query') ||
        i.includes('/lib/transport'),
    )
    expect(offending).toEqual([])
  })
})

describe('organisms', () => {
  const files = sourceFilesIn('organisms')

  it('コンポーネントが存在する', () => {
    expect(files.length).toBeGreaterThan(0)
  })

  // organisms は通信を知ってよい。知ってはいけないのはルーティング。
  // ルートに対応するのは pages の役割で、organisms は置かれた場所を知らない。
  it.each(files)('$path はルーティングを import しない', ({ source }) => {
    const offending = importsOf(source).filter((i) => i.includes('react-router'))
    expect(offending).toEqual([])
  })

  it.each(files)('$path は templates と pages を import しない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) => i.includes('templates') || i.includes('/pages/'),
    )
    expect(offending).toEqual([])
  })
})

describe('templates', () => {
  const files = sourceFilesIn('templates')

  // templates は配置だけを決める。実データを取りに行かない。
  it.each(files)('$path はデータ取得を行わない', ({ source }) => {
    const offending = importsOf(source).filter(
      (i) => i.includes('@tanstack/react-query') || i.includes('/lib/transport'),
    )
    expect(offending).toEqual([])
  })
})
