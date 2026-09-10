import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { WorldDeleteDialog } from './WorldDeleteDialog'

function renderDialog(onConfirm = vi.fn(), onCancel = vi.fn()) {
  render(<WorldDeleteDialog name="world" onConfirm={onConfirm} onCancel={onCancel} />)
  return { onConfirm, onCancel }
}

describe('WorldDeleteDialog', () => {
  /**
   * REQ-123。押し間違いがそのままデータの喪失になる。
   * 名前が完全に一致するまで実行できない。
   */
  it('名前を打ち込むまで実行できない', async () => {
    renderDialog()

    expect(screen.getByRole('button', { name: '削除する' })).toBeDisabled()

    await userEvent.type(screen.getByRole('textbox'), 'wor')
    expect(screen.getByRole('button', { name: '削除する' })).toBeDisabled()

    await userEvent.type(screen.getByRole('textbox'), 'ld')
    expect(screen.getByRole('button', { name: '削除する' })).toBeEnabled()
  })

  it.each([
    ['大文字小文字違い', 'World'],
    ['前後に空白', ' world '],
    ['余分な文字', 'world2'],
  ])('%s では実行できない', async (_name, typed) => {
    renderDialog()

    await userEvent.type(screen.getByRole('textbox'), typed)
    expect(screen.getByRole('button', { name: '削除する' })).toBeDisabled()
  })

  it('一致したら打ち込んだ名前を渡す', async () => {
    const { onConfirm } = renderDialog()

    await userEvent.type(screen.getByRole('textbox'), 'world')
    await userEvent.click(screen.getByRole('button', { name: '削除する' }))

    expect(onConfirm).toHaveBeenCalledWith('world')
  })

  // すぐには消えないことを、実行する前に伝える。
  it('退避先を明示する', () => {
    renderDialog()
    expect(screen.getByText(/deleted-日時/)).toBeInTheDocument()
  })

  it('やめられる', async () => {
    const { onCancel, onConfirm } = renderDialog()

    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))
    expect(onCancel).toHaveBeenCalled()
    expect(onConfirm).not.toHaveBeenCalled()
  })

  // 操作の実行中は押せない。サーバー側の排他を画面に映す。
  it('無効化できる', () => {
    render(
      <WorldDeleteDialog name="world" disabled onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByRole('button', { name: 'やめる' })).toBeDisabled()
    expect(screen.getByRole('textbox')).toBeDisabled()
  })
})
