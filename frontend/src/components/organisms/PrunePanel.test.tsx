import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { PrunePanel } from './PrunePanel'

function renderPanel(props: Partial<Parameters<typeof PrunePanel>[0]> = {}) {
  const onPreview = vi.fn()
  const onApply = vi.fn()
  render(<PrunePanel preview={null} onPreview={onPreview} onApply={onApply} {...props} />)
  return { onPreview, onApply }
}

/**
 * 保持ポリシーの手動適用。dry_run で対象を見せてから消す。
 * 「押したら何が消えるか分からない」状態で削除させない。
 */
describe('PrunePanel', () => {
  it('まず対象を確認させる', async () => {
    const { onPreview, onApply } = renderPanel()

    expect(screen.queryByRole('button', { name: '削除する' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '対象を確認' }))

    expect(onPreview).toHaveBeenCalled()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('確認したファイル名を並べてから削除させる', async () => {
    const { onApply } = renderPanel({ preview: ['old-1.zip', 'old-2.zip'] })

    expect(screen.getByText('old-1.zip')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '削除する' }))

    expect(onApply).toHaveBeenCalled()
  })

  it('対象が無いときは削除させない', () => {
    renderPanel({ preview: [] })

    expect(screen.getByText(/削除するものはありません/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '削除する' })).not.toBeInTheDocument()
  })

  it('無効化できる', () => {
    renderPanel({ disabled: true })
    expect(screen.getByRole('button', { name: '対象を確認' })).toBeDisabled()
  })
})
