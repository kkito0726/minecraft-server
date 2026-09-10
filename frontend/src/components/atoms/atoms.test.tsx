import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Badge, ProgressBar, Spinner } from './index'

describe('Badge', () => {
  it.each(['neutral', 'ok', 'warn', 'danger'] as const)('%s の見た目で描画する', (tone) => {
    render(<Badge tone={tone}>ラベル</Badge>)
    expect(screen.getByText('ラベル')).toBeInTheDocument()
  })

  it('既定は neutral', () => {
    render(<Badge>既定</Badge>)
    expect(screen.getByText('既定')).toBeInTheDocument()
  })
})

describe('ProgressBar', () => {
  it('総量が分かるときは現在値を読み上げる', () => {
    render(<ProgressBar done={30} total={100} label="展開" />)

    const bar = screen.getByRole('progressbar', { name: '展開' })
    expect(bar).toHaveAttribute('aria-valuenow', '30')
    expect(bar).toHaveAttribute('aria-valuemax', '100')
  })

  // 総量が取れないアーカイブがある。0 で割って NaN を出さない。
  it('総量が不明なら値を持たない', () => {
    render(<ProgressBar done={30} total={0} label="展開" />)

    const bar = screen.getByRole('progressbar', { name: '展開' })
    expect(bar).not.toHaveAttribute('aria-valuenow')
  })

  it.each([
    ['総量を超えた', 200, 100],
    ['負の値', -5, 100],
  ])('%s 場合も壊れない', (_name, done, total) => {
    render(<ProgressBar done={done} total={total} label="展開" />)
    expect(screen.getByRole('progressbar', { name: '展開' })).toBeInTheDocument()
  })
})

describe('Spinner', () => {
  it('既定では処理中であることを読み上げる', () => {
    render(<Spinner />)
    expect(screen.getByRole('status', { name: '処理中' })).toBeInTheDocument()
  })

  it('説明を差し替えられる', () => {
    render(<Spinner label="確認しています" />)
    expect(screen.getByRole('status', { name: '確認しています' })).toBeInTheDocument()
  })

  /**
   * すでに状況を読み上げている領域の中に置くときは隠す。
   * ライブリージョンが入れ子になると同じ進捗が二重に読み上げられる。
   */
  it('飾りとして使うときは支援技術から隠す', () => {
    render(<Spinner decorative />)
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
