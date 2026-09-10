import { Code, ConnectError } from '@connectrpc/connect'
import { describe, expect, it } from 'vitest'

import { createQueryClient } from './queryClient'

function retryOf(client: ReturnType<typeof createQueryClient>) {
  const retry = client.getDefaultOptions().queries?.retry
  if (typeof retry !== 'function') {
    throw new Error('retry が関数ではありません')
  }
  return retry
}

describe('createQueryClient', () => {
  // 認可の失敗は何度やっても同じ結果になる。
  // 再試行すると、間違ったトークンのまま無駄な往復が増えるだけ。
  it('認証の失敗は再試行しない', () => {
    const retry = retryOf(createQueryClient())
    expect(retry(0, new ConnectError('', Code.Unauthenticated))).toBe(false)
  })

  it('一時的な失敗は数回だけ再試行する', () => {
    const retry = retryOf(createQueryClient())
    expect(retry(0, new ConnectError('', Code.Unavailable))).toBe(true)
    expect(retry(1, new ConnectError('', Code.Unavailable))).toBe(true)
    expect(retry(2, new ConnectError('', Code.Unavailable))).toBe(false)
  })

  // 起動・復元・削除には副作用がある。自動でやり直すと二重に走る。
  it('変更系は再試行しない', () => {
    expect(createQueryClient().getDefaultOptions().mutations?.retry).toBe(false)
  })
})
