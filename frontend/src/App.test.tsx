import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { App } from './App'

// 疎通の確認だけを差し替える。ここでの関心は認証の有無で
// 画面が切り替わることと、ルーティングが動くこと。
const verify = vi.hoisted(() => vi.fn(async () => {}))
vi.mock('./features/auth/verify', () => ({ verifyToken: verify }))

// サーバーの状態取得も差し替える。ここでの関心は認証と画面の切り替え。
const getStatus = vi.hoisted(() => vi.fn(async () => ({ containerState: 0, healthy: false })))
vi.mock('./features/server/client', () => ({
  serverClient: { getStatus, startServer: vi.fn(), stopServer: vi.fn(), restartServer: vi.fn() },
}))

const listWorlds = vi.hoisted(() =>
  vi.fn(async () => ({ worlds: [], quarantines: [], activeLevel: 'world' })),
)
vi.mock('./features/worlds/client', () => ({
  worldClient: { listWorlds, purgeQuarantine: vi.fn() },
}))

// 進捗の購読も差し替える。ここでの関心は認証と画面の切り替えであって、
// 実際の通信ではない。差し替えないと jsdom から本物の fetch が飛ぶ。
const active = vi.hoisted(() => vi.fn(async () => null))
vi.mock('./features/operations/watch', () => ({
  createOperationSource: () => ({
    active,
    watch: () => ({
      // eslint-disable-next-line require-yield
      async *[Symbol.asyncIterator]() {
        return
      },
    }),
  }),
}))

afterEach(() => {
  window.localStorage.clear()
  window.history.pushState({}, '', '/')
  vi.clearAllMocks()
})

describe('App', () => {
  it('トークンが無ければ入力を求め、画面は出さない', () => {
    render(<App />)

    expect(screen.getByLabelText(/トークン/)).toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('トークンを入れると画面が出る', async () => {
    render(<App />)

    await userEvent.type(screen.getByLabelText(/トークン/), 'a'.repeat(64))
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    expect(await screen.findByRole('heading', { name: 'サーバーの状態' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'ワールド' })).toBeInTheDocument()
  })

  it('ナビゲーションで画面を切り替えられる', async () => {
    window.localStorage.setItem('mcadmin.token', 'a'.repeat(64))
    render(<App />)

    // 見出しで確かめる。ナビゲーションのリンクと同じ文言なので
    // 単なるテキスト検索では区別できない。
    await userEvent.click(await screen.findByRole('link', { name: 'バックアップ' }))
    expect(await screen.findByRole('heading', { name: 'バックアップ' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('link', { name: 'ワールド' }))
    expect(await screen.findByRole('heading', { name: 'ワールド' })).toBeInTheDocument()
  })

  // 知らないパスで真っ白にしない。リロードで /backups を開いたときも同じ経路を通る。
  it('知らないパスはサーバーの画面へ寄せる', async () => {
    window.localStorage.setItem('mcadmin.token', 'a'.repeat(64))
    window.history.pushState({}, '', '/存在しない')
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'サーバーの状態' })).toBeInTheDocument()
  })
})
