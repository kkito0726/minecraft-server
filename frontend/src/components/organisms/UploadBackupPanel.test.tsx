import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { UploadBackupPanel } from './UploadBackupPanel'

function renderPanel(props: Partial<Parameters<typeof UploadBackupPanel>[0]> = {}) {
  const onUpload = vi.fn()
  render(<UploadBackupPanel progress={null} onUpload={onUpload} {...props} />)
  return { onUpload }
}

const zip = () => new File(['zip'], 'MyWorld.zip', { type: 'application/zip' })

describe('UploadBackupPanel', () => {
  it('選ぶまで送れない', () => {
    renderPanel()
    expect(screen.getByRole('button', { name: '取り込む' })).toBeDisabled()
  })

  it('選んだファイルを送る', async () => {
    const { onUpload } = renderPanel()
    const file = zip()

    await userEvent.upload(screen.getByLabelText('取り込む zip'), file)
    await userEvent.click(screen.getByRole('button', { name: '取り込む' }))

    expect(onUpload).toHaveBeenCalledWith(file)
  })

  it('選んだファイルの名前を出す', async () => {
    renderPanel()

    await userEvent.upload(screen.getByLabelText('取り込む zip'), zip())
    expect(screen.getByText(/MyWorld\.zip/)).toBeInTheDocument()
  })

  // 30MB のワールドを無反応で待たせない。
  it('送信中は進み具合を出し、二重に送らせない', () => {
    renderPanel({ progress: 0.4 })

    expect(screen.getByRole('progressbar')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '送信しています…' })).toBeDisabled()
  })

  it('失敗した理由を出す', () => {
    renderPanel({ error: 'level.dat が zip の直下にあります。' })
    expect(screen.getByRole('alert')).toHaveTextContent('level.dat が zip の直下にあります。')
  })

  it('取り込めたことを伝える', () => {
    renderPanel({ notice: 'MyWorld を取り込みました。' })
    expect(screen.getByText(/取り込みました/)).toBeInTheDocument()
  })

  // 取り込んだだけではワールドは変わらない。誤解させない。
  it('まだ差し替わらないことを伝える', () => {
    renderPanel()
    expect(screen.getByText(/まだ差し替わりません/)).toBeInTheDocument()
  })

  it('操作の実行中は無効化できる', () => {
    renderPanel({ disabled: true })
    expect(screen.getByLabelText('取り込む zip')).toBeDisabled()
  })
})
