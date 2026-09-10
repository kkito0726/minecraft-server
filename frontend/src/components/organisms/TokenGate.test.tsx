import { Code, ConnectError } from '@connectrpc/connect'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { readToken, writeToken } from '../../features/auth/token'
import { TokenGate } from './TokenGate'

const VALID = 'a'.repeat(64)

afterEach(() => {
  window.localStorage.clear()
  vi.restoreAllMocks()
})

/** verify は「このトークンでサーバーに触れるか」を確かめる関数。 */
function renderGate(verify: (token: string) => Promise<void>) {
  return render(
    <TokenGate verify={verify}>
      <div>入場できました</div>
    </TokenGate>,
  )
}

describe('未設定のとき', () => {
  it('入力を求める', () => {
    renderGate(async () => {})

    expect(screen.getByLabelText(/トークン/)).toBeInTheDocument()
    expect(screen.queryByText('入場できました')).not.toBeInTheDocument()
  })

  // 32 文字未満はサーバーに送る前に断る。無駄な往復をしない。
  it('短すぎるトークンは送らずに断る', async () => {
    const verify = vi.fn(async () => {})
    renderGate(verify)

    await userEvent.type(screen.getByLabelText(/トークン/), 'short')
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    expect(await screen.findByText(/32 文字以上/)).toBeInTheDocument()
    expect(verify).not.toHaveBeenCalled()
  })

  it('正しいトークンで入場でき、保存される', async () => {
    renderGate(async () => {})

    await userEvent.type(screen.getByLabelText(/トークン/), VALID)
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    expect(await screen.findByText('入場できました')).toBeInTheDocument()
    expect(readToken()).toBe(VALID)
  })

  /**
   * 入力された値そのものを verify へ渡す。保存はその後。
   * 渡さずに「保存済みのトークンで確かめる」実装にすると、
   * まだ保存していないので検証が必ず失敗する。
   */
  it('入力されたトークンを確認に渡す', async () => {
    const verify = vi.fn(async () => {})
    renderGate(verify)

    await userEvent.type(screen.getByLabelText(/トークン/), VALID)
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    await waitFor(() => expect(verify).toHaveBeenCalledWith(VALID))
  })

  /**
   * 誤ったトークンは、認証の失敗として画面に出す。
   *
   * ここを「トークンを消して再読み込み」の共通処理に任せると、
   * 入力するたびに画面が点滅するだけで理由が表示されない。
   */
  it('誤ったトークンは理由を出して入力欄に留まる', async () => {
    const verify = vi.fn(async () => {
      throw new ConnectError('', Code.Unauthenticated)
    })
    renderGate(verify)

    await userEvent.type(screen.getByLabelText(/トークン/), VALID)
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    expect(await screen.findByText(/認証できませんでした/)).toBeInTheDocument()
    expect(screen.queryByText('入場できました')).not.toBeInTheDocument()
    // 通らなかったトークンは保存しない。次に開いたときも同じ失敗を繰り返す意味がない。
    expect(readToken()).toBe('')
  })

  it('サーバーに繋がらないときは接続の問題だと分かる', async () => {
    renderGate(async () => {
      throw new ConnectError('', Code.Unavailable)
    })

    await userEvent.type(screen.getByLabelText(/トークン/), VALID)
    await userEvent.click(screen.getByRole('button', { name: /入る/ }))

    expect(await screen.findByText(/接続できません/)).toBeInTheDocument()
  })
})

describe('保存済みのとき', () => {
  it('確認が通れば入力を求めずに入場する', async () => {
    writeToken(VALID)
    const verify = vi.fn(async () => {})
    renderGate(verify)

    expect(await screen.findByText('入場できました')).toBeInTheDocument()
    expect(verify).toHaveBeenCalledWith(VALID)
  })

  /**
   * 保存されたトークンが失効している場合。
   * .env の ADMIN_TOKEN を入れ替えたときに必ず通る経路。
   */
  it('確認に失敗したら消して入力を求める', async () => {
    writeToken(VALID)
    renderGate(async () => {
      throw new ConnectError('', Code.Unauthenticated)
    })

    expect(await screen.findByLabelText(/トークン/)).toBeInTheDocument()
    await waitFor(() => expect(readToken()).toBe(''))
  })

  it('確認中は入力欄も本体も出さない', async () => {
    writeToken(VALID)
    let release = () => {}
    const blocked = new Promise<void>((resolve) => {
      release = resolve
    })
    renderGate(() => blocked)

    expect(screen.queryByLabelText(/トークン/)).not.toBeInTheDocument()
    expect(screen.queryByText('入場できました')).not.toBeInTheDocument()

    release()
    expect(await screen.findByText('入場できました')).toBeInTheDocument()
  })
})
