import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { BackupMode } from '../../gen/mcadmin/v1/backup_pb'
import { CreateBackupDialog } from './CreateBackupDialog'

function renderDialog() {
  const onSubmit = vi.fn()
  const onCancel = vi.fn()
  render(<CreateBackupDialog onSubmit={onSubmit} onCancel={onCancel} />)
  return { onSubmit, onCancel }
}

describe('CreateBackupDialog', () => {
  // 既定は HOT。停止を伴う COLD を既定にすると、軽い気持ちの
  // バックアップがサーバーの再起動になる。
  it('既定は稼働したまま取る', async () => {
    const { onSubmit } = renderDialog()

    await userEvent.click(screen.getByRole('button', { name: '取得する' }))
    expect(onSubmit).toHaveBeenCalledWith({ mode: BackupMode.HOT, note: '' })
  })

  it('停止して取ることも選べる', async () => {
    const { onSubmit } = renderDialog()

    await userEvent.click(screen.getByRole('radio', { name: /停止してから/ }))
    await userEvent.click(screen.getByRole('button', { name: '取得する' }))

    expect(onSubmit).toHaveBeenCalledWith({ mode: BackupMode.COLD, note: '' })
  })

  it('停止を伴うことを選んだときに伝える', async () => {
    renderDialog()

    await userEvent.click(screen.getByRole('radio', { name: /停止してから/ }))
    expect(screen.getByRole('alert')).toHaveTextContent(/停止/)
  })

  it('メモを添えられる', async () => {
    const { onSubmit } = renderDialog()

    await userEvent.type(screen.getByLabelText(/メモ/), 'before-update')
    await userEvent.click(screen.getByRole('button', { name: '取得する' }))

    expect(onSubmit).toHaveBeenCalledWith({ mode: BackupMode.HOT, note: 'before-update' })
  })

  // メモはファイル名の一部になり、使えない文字は落ちる。
  // 押したあとに気づくのでは遅いので、結果を先に見せる。
  it('ファイル名に残る形を先に見せる', async () => {
    renderDialog()

    await userEvent.type(screen.getByLabelText(/メモ/), 'before update')
    expect(screen.getByText('before-update')).toBeInTheDocument()
  })

  it('日本語のメモが残らないことを伝える', async () => {
    renderDialog()

    await userEvent.type(screen.getByLabelText(/メモ/), 'アップデート前')
    expect(screen.getByText(/ファイル名には残りません/)).toBeInTheDocument()
  })

  it('やめられる', async () => {
    const { onCancel, onSubmit } = renderDialog()

    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))
    expect(onCancel).toHaveBeenCalled()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('無効化できる', () => {
    render(<CreateBackupDialog disabled onSubmit={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.getByRole('button', { name: '取得する' })).toBeDisabled()
  })
})
