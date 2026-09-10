import { afterEach, describe, expect, it, vi } from 'vitest'

import { clearToken, isValidToken, readToken, tokenSchema, writeToken } from './token'

afterEach(() => {
  // 差し替えを先に戻す。偽物の localStorage に clear() は無い。
  vi.unstubAllGlobals()
  window.localStorage.clear()
})

describe('tokenSchema', () => {
  // ADMIN_TOKEN は openssl rand -hex 32 で作った 64 文字を想定している。
  // バックエンドが 32 文字未満を起動時に拒否するので、画面も同じ線で断る。
  it.each([
    ['32 文字ちょうど', 'a'.repeat(32), true],
    ['64 文字', 'b'.repeat(64), true],
    ['31 文字', 'c'.repeat(31), false],
    ['空', '', false],
  ])('%s → %s', (_name, value, valid) => {
    expect(tokenSchema.safeParse(value).success).toBe(valid)
  })

  it('短すぎるときは理由を日本語で返す', () => {
    const result = tokenSchema.safeParse('short')
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]?.message).toContain('32')
    }
  })

  it('前後の空白は取り除く', () => {
    const result = tokenSchema.safeParse(`  ${'a'.repeat(64)}  `)
    expect(result.success).toBe(true)
    if (result.success) {
      expect(result.data).toBe('a'.repeat(64))
    }
  })
})

describe('isValidToken', () => {
  it('長さだけで判定する', () => {
    expect(isValidToken('a'.repeat(32))).toBe(true)
    expect(isValidToken('a'.repeat(31))).toBe(false)
  })
})

describe('保存と読み出し', () => {
  it('書いたものが読める', () => {
    writeToken('a'.repeat(64))
    expect(readToken()).toBe('a'.repeat(64))
  })

  it('未設定なら空文字', () => {
    expect(readToken()).toBe('')
  })

  it('消せる', () => {
    writeToken('a'.repeat(64))
    clearToken()
    expect(readToken()).toBe('')
  })

  // プライベートウィンドウでは localStorage の参照そのものが例外を投げる。
  // トークンが保持できないだけで画面が真っ白になってはならない。
  it('localStorage が使えなくても例外にしない', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('保存領域が使えません')
      },
      setItem: () => {
        throw new Error('保存領域が使えません')
      },
      removeItem: () => {
        throw new Error('保存領域が使えません')
      },
    })

    expect(() => writeToken('a'.repeat(64))).not.toThrow()
    expect(readToken()).toBe('')
    expect(() => clearToken()).not.toThrow()
  })
})
