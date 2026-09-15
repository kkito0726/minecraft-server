import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { OperationSource } from '../features/operations'
import { OperationKind, OperationState } from '../gen/mcadmin/v1/common_pb'
import { OperationSchema } from '../gen/mcadmin/v1/operation_pb'
import {
  ContainerState,
  Difficulty,
  GetGameSettingsResponseSchema,
  GetStatusResponseSchema,
  UpdateGameSettingsResponseSchema,
} from '../gen/mcadmin/v1/server_pb'
import { withProviders } from '../test/providers'
import { SettingsPage } from './SettingsPage'

const getStatus = vi.hoisted(() => vi.fn())
const getGameSettings = vi.hoisted(() => vi.fn())
const updateGameSettings = vi.hoisted(() => vi.fn())

vi.mock('../features/server/client', () => ({
  serverClient: { getStatus, getGameSettings, updateGameSettings },
}))

afterEach(() => {
  vi.clearAllMocks()
})

// 進捗の購読は使わない。本物の通信が jsdom から飛ばないようにする。
const idleSource: OperationSource = {
  active: async () => null,
  watch: () => ({
    // eslint-disable-next-line require-yield
    async *[Symbol.asyncIterator]() {
      return
    },
  }),
}

const current = {
  difficulty: Difficulty.NORMAL,
  motd: 'ようこそ',
  maxPlayers: 5,
  viewDistance: 7,
  simulationDistance: 5,
}

function arrange({ running = true, online = 0 } = {}) {
  getStatus.mockResolvedValue(
    create(GetStatusResponseSchema, {
      containerState: running ? ContainerState.RUNNING : ContainerState.EXITED,
      onlinePlayers: online,
      maxPlayers: 5,
    }),
  )
  getGameSettings.mockResolvedValue(create(GetGameSettingsResponseSchema, { settings: current }))
  render(withProviders(<SettingsPage />, idleSource))
}

describe('SettingsPage', () => {
  it('.env のゲーム設定を表示する', async () => {
    arrange()

    expect(await screen.findByRole('heading', { name: 'ゲーム設定' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /ノーマル/ })).toBeChecked()
    expect(screen.getByLabelText('最大人数')).toHaveValue('5')
  })

  it('保存だけなら、次の起動で反映されると伝える', async () => {
    updateGameSettings.mockResolvedValue(create(UpdateGameSettingsResponseSchema, { settings: current }))
    arrange()

    await userEvent.click(await screen.findByRole('radio', { name: /ハード/ }))
    await userEvent.click(screen.getByRole('button', { name: '保存だけする' }))

    expect(updateGameSettings).toHaveBeenCalledWith({
      settings: { ...current, difficulty: Difficulty.HARD },
      applyNow: false,
    })
    expect(await screen.findByText(/保存しました。まだ反映していません/)).toBeInTheDocument()
  })

  // 作り直しが始まったら、進捗は上の帯が伝える。ページで重ねて言わない。
  it('今すぐ反映して操作が始まったら、保存の案内は出さない', async () => {
    updateGameSettings.mockResolvedValue(
      create(UpdateGameSettingsResponseSchema, {
        settings: current,
        operation: create(OperationSchema, {
          id: 'op-1',
          kind: OperationKind.SERVER_APPLY_SETTINGS,
          state: OperationState.PENDING,
          stepTotal: 5,
        }),
      }),
    )
    arrange()

    await userEvent.click(await screen.findByRole('radio', { name: /イージー/ }))
    await userEvent.click(screen.getByRole('button', { name: '保存して今すぐ反映' }))

    expect(updateGameSettings).toHaveBeenCalledWith(expect.objectContaining({ applyNow: true }))
    // 応答の処理が終わる前に判定すると、案内が出ないのは当たり前になってしまう。
    // 保存後の読み直しと、操作の開始による入力欄の無効化を待ってから確かめる。
    await waitFor(() => expect(getGameSettings).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.getByLabelText('最大人数')).toBeDisabled())
    expect(screen.queryByText(/保存しました/)).not.toBeInTheDocument()
  })

  it('停止中は今すぐ反映の口を出さない', async () => {
    arrange({ running: false })

    expect(await screen.findByRole('button', { name: '保存する' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '保存して今すぐ反映' })).not.toBeInTheDocument()
  })

  // 画面を開いている間に .env が手で書き換えられた。理由を伏せずに伝える。
  it('保存に失敗したら理由を出す', async () => {
    updateGameSettings.mockRejectedValue(
      new ConnectError('.env が外部から変更されました。読み込み直してから保存してください', Code.Aborted),
    )
    arrange()

    await userEvent.click(await screen.findByRole('radio', { name: /ハード/ }))
    await userEvent.click(screen.getByRole('button', { name: '保存だけする' }))

    expect(await screen.findByText(/外部から変更されました/)).toBeInTheDocument()
  })

  it('読み込めなければ理由を出す', async () => {
    getStatus.mockResolvedValue(create(GetStatusResponseSchema, {}))
    getGameSettings.mockRejectedValue(new ConnectError('down', Code.Unavailable))
    render(withProviders(<SettingsPage />, idleSource))

    expect(await screen.findByText(/down/)).toBeInTheDocument()
  })
})
