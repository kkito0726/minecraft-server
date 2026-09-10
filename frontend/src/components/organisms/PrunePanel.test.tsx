import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { PrunePanel } from './PrunePanel'

function renderPanel(props: Partial<Parameters<typeof PrunePanel>[0]> = {}) {
  const onPreview = vi.fn()
  const onApply = vi.fn()
  render(<PrunePanel result={null} onPreview={onPreview} onApply={onApply} {...props} />)
  return { onPreview, onApply }
}

const applyButton = () => screen.queryByRole('button', { name: '削除する' })

/**
 * 保持ポリシーの手動適用。dry_run で対象を見せてから消す。
 * 「押したら何が消えるか分からない」状態で削除させない。
 */
describe('PrunePanel', () => {
  it('まず対象を確認させる', async () => {
    const { onPreview, onApply } = renderPanel()

    expect(applyButton()).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '対象を確認' }))

    expect(onPreview).toHaveBeenCalled()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('確認したファイル名を並べてから削除させる', async () => {
    const { onApply } = renderPanel({
      result: { kind: 'preview', ids: ['old-1.zip', 'old-2.zip'] },
    })

    expect(screen.getByText('old-1.zip')).toBeInTheDocument()
    await userEvent.click(applyButton()!)

    expect(onApply).toHaveBeenCalled()
  })

  it('対象が無いときは削除させない', () => {
    renderPanel({ result: { kind: 'preview', ids: [] } })

    expect(screen.getByText(/削除するものはありません/)).toBeInTheDocument()
    expect(applyButton()).not.toBeInTheDocument()
  })

  /**
   * 消したあとは「消した」と書く。確認した集合と実際に消えた集合は
   * 食い違いうる（確認のあとに取得が走れば保持ポリシーが先に適用される）。
   * 予定を貼ったままにすると、消えていないものを消したように見せてしまう。
   */
  it('削除したあとは実際に消えたものを過去形で出す', () => {
    renderPanel({ result: { kind: 'applied', ids: ['old-1.zip'] } })

    expect(screen.getByText(/削除しました/)).toBeInTheDocument()
    expect(screen.getByText('old-1.zip')).toBeInTheDocument()
  })

  // 消したあとの一覧に対して押せると、確認していない集合を消すことになる。
  it('削除したあとは続けて削除できない', () => {
    renderPanel({ result: { kind: 'applied', ids: ['old-1.zip'] } })
    expect(applyButton()).not.toBeInTheDocument()
  })

  it('何も消えなかったこともそう書く', () => {
    renderPanel({ result: { kind: 'applied', ids: [] } })
    expect(screen.getByText(/削除したものはありません/)).toBeInTheDocument()
  })

  it('無効化できる', () => {
    renderPanel({ disabled: true })
    expect(screen.getByRole('button', { name: '対象を確認' })).toBeDisabled()
  })
})
