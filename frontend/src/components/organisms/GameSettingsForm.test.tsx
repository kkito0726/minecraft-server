import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { GameSettingsValues } from '../../features/settings'
import { Difficulty } from '../../gen/mcadmin/v1/server_pb'
import { GameSettingsForm } from './GameSettingsForm'

const current: GameSettingsValues = {
  difficulty: Difficulty.NORMAL,
  motd: 'ようこそ',
  maxPlayers: 5,
  viewDistance: 7,
  simulationDistance: 5,
}

function renderForm(props: Partial<Parameters<typeof GameSettingsForm>[0]> = {}) {
  const onSave = vi.fn()
  render(
    <GameSettingsForm
      settings={current}
      warnings={[]}
      running
      onlinePlayers={0}
      onSave={onSave}
      {...props}
    />,
  )
  return { onSave }
}

const saveOnly = () => screen.getByRole('button', { name: '保存だけする' })
const applyNow = () => screen.getByRole('button', { name: '保存して今すぐ反映' })

describe('GameSettingsForm', () => {
  it('今の設定を表示する', () => {
    renderForm()

    expect(screen.getByRole('radio', { name: /ノーマル/ })).toBeChecked()
    expect(screen.getByLabelText(/MOTD/)).toHaveValue('ようこそ')
    expect(screen.getByLabelText('最大人数')).toHaveValue('5')
    expect(screen.getByLabelText('描画距離')).toHaveValue('7')
    expect(screen.getByLabelText('シミュレーション距離')).toHaveValue('5')
  })

  it('変更がなければ保存できない', () => {
    renderForm()

    expect(saveOnly()).toBeDisabled()
    expect(applyNow()).toBeDisabled()
  })

  it('保存だけなら反映しない', async () => {
    const { onSave } = renderForm()

    await userEvent.click(screen.getByRole('radio', { name: /ハード/ }))
    await userEvent.click(saveOnly())

    expect(onSave).toHaveBeenCalledWith({ ...current, difficulty: Difficulty.HARD }, false)
  })

  it('誰もいなければ、すぐに反映する', async () => {
    const { onSave } = renderForm()

    await userEvent.clear(screen.getByLabelText('最大人数'))
    await userEvent.type(screen.getByLabelText('最大人数'), '8')
    await userEvent.click(applyNow())

    expect(onSave).toHaveBeenCalledWith({ ...current, maxPlayers: 8 }, true)
  })

  // 作り直しは接続を切る。人がいるときは一度確かめる。
  it('人がいれば、切断することを確かめてから反映する', async () => {
    const { onSave } = renderForm({ onlinePlayers: 2 })

    await userEvent.click(screen.getByRole('radio', { name: /イージー/ }))
    await userEvent.click(applyNow())

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText(/2 人が接続しています/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '切断して反映する' }))
    expect(onSave).toHaveBeenCalledWith({ ...current, difficulty: Difficulty.EASY }, true)
  })

  it('確認はやめられる', async () => {
    const { onSave } = renderForm({ onlinePlayers: 2 })

    await userEvent.click(screen.getByRole('radio', { name: /イージー/ }))
    await userEvent.click(applyNow())
    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))

    expect(screen.queryByText(/2 人が接続しています/)).not.toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()
  })

  // 止まっているサーバーを勝手に起動しない。反映の口そのものを出さない。
  it('停止中は保存だけを出す', () => {
    renderForm({ running: false })

    expect(screen.queryByRole('button', { name: '保存して今すぐ反映' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存する' })).toBeInTheDocument()
    expect(screen.getByText(/次の起動で反映/)).toBeInTheDocument()
  })

  it('規則外の値では保存できず、理由を出す', async () => {
    renderForm()

    await userEvent.clear(screen.getByLabelText('シミュレーション距離'))
    await userEvent.type(screen.getByLabelText('シミュレーション距離'), '9')

    expect(saveOnly()).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent(/描画距離（7）以下/)
  })

  it('Pi での目安を超えたら注意を出すが、保存は止めない', async () => {
    renderForm()

    await userEvent.clear(screen.getByLabelText('描画距離'))
    await userEvent.type(screen.getByLabelText('描画距離'), '12')

    expect(screen.getByText(/描画距離が 10 を超えています/)).toBeInTheDocument()
    expect(saveOnly()).toBeEnabled()
  })

  it('.env を読めなかった項目を伝える', () => {
    renderForm({ warnings: ['MC_VIEW_DISTANCE の値 "abc" を読めなかったため、既定の 7 を表示しています'] })

    expect(screen.getByText(/MC_VIEW_DISTANCE/)).toBeInTheDocument()
  })

  it('無効化できる', () => {
    renderForm({ disabled: true })

    expect(screen.getByLabelText('最大人数')).toBeDisabled()
  })
})
