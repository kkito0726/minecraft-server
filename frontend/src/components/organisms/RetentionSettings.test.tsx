import { create } from '@bufbuild/protobuf'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { RetentionPolicySchema } from '../../gen/mcadmin/v1/backup_pb'
import { RetentionSettings } from './RetentionSettings'

const policy = create(RetentionPolicySchema, { keepCount: 10, keepDays: 0 })

function renderSettings(props: Partial<Parameters<typeof RetentionSettings>[0]> = {}) {
  const onSave = vi.fn()
  render(<RetentionSettings policy={policy} onSave={onSave} {...props} />)
  return { onSave }
}

const countInput = () => screen.getByLabelText(/世代/)
const daysInput = () => screen.getByLabelText(/日数/)
const saveButton = () => screen.getByRole('button', { name: '保存する' })

describe('RetentionSettings', () => {
  it('今の設定を表示する', () => {
    renderSettings()
    expect(countInput()).toHaveValue('10')
    expect(daysInput()).toHaveValue('0')
  })

  it('変更を保存できる', async () => {
    const { onSave } = renderSettings()

    await userEvent.clear(countInput())
    await userEvent.type(countInput(), '5')
    await userEvent.clear(daysInput())
    await userEvent.type(daysInput(), '30')
    await userEvent.click(saveButton())

    expect(onSave).toHaveBeenCalledWith({ keepCount: 5, keepDays: 30 })
  })

  /**
   * EDGE-104。0 世代はバックアップの全損を意味する。
   * バックエンドは invalid_argument で拒むが、往復させる理由がない。
   */
  it('0 世代では保存できない', async () => {
    renderSettings()

    await userEvent.clear(countInput())
    await userEvent.type(countInput(), '0')

    expect(saveButton()).toBeDisabled()
    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('変更がなければ保存できない', () => {
    renderSettings()
    expect(saveButton()).toBeDisabled()
  })

  // EDGE-103。設定がどうであれ最新の 1 世代は残る。
  it('最低 1 世代は残ることを伝える', () => {
    renderSettings()
    expect(screen.getByText(/最も新しい 1 世代/)).toBeInTheDocument()
  })

  it('無効化できる', () => {
    renderSettings({ disabled: true })
    expect(countInput()).toBeDisabled()
  })
})
