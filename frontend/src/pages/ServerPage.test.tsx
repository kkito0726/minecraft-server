import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { OperationKind, OperationState } from '../gen/mcadmin/v1/common_pb'
import { OperationSchema, WatchOperationResponseSchema } from '../gen/mcadmin/v1/operation_pb'
import type { Operation } from '../gen/mcadmin/v1/operation_pb'
import {
  ContainerState,
  GetStatusResponseSchema,
  SavingState,
} from '../gen/mcadmin/v1/server_pb'
import type { OperationSource } from '../features/operations'
import { withProviders } from '../test/providers'
import { ServerPage } from './ServerPage'

const getStatus = vi.hoisted(() => vi.fn())
const startServer = vi.hoisted(() => vi.fn())
const stopServer = vi.hoisted(() => vi.fn())
const restartServer = vi.hoisted(() => vi.fn())

vi.mock('../features/server/client', () => ({
  serverClient: { getStatus, startServer, stopServer, restartServer },
}))

afterEach(() => {
  vi.clearAllMocks()
})

type StatusOverrides = Extract<
  MessageInitShape<typeof GetStatusResponseSchema>,
  { $typeName?: never }
>

function status(overrides: StatusOverrides = {}) {
  return create(GetStatusResponseSchema, {
    containerState: ContainerState.RUNNING,
    healthy: true,
    configuredVersion: '26.2',
    activeLevel: 'world',
    activeWorldVersion: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
    onlinePlayers: 0,
    maxPlayers: 5,
    savingState: SavingState.ASSUMED_ON,
    ...overrides,
  })
}

function op(overrides: MessageInitShape<typeof OperationSchema> = {}): Operation {
  return create(OperationSchema, {
    id: 'op-1',
    kind: OperationKind.SERVER_RESTART,
    state: OperationState.PENDING,
    stepTotal: 4,
    ...overrides,
  })
}

/** 進行中の操作が無い購読元。 */
const idleSource: OperationSource = {
  active: async () => null,
  watch: () => ({
    // eslint-disable-next-line require-yield
    async *[Symbol.asyncIterator]() {
      return
    },
  }),
}

function renderPage(source: OperationSource = idleSource) {
  return render(withProviders(<ServerPage />, source))
}

describe('状態の表示', () => {
  it('REQ-001 の項目を 1 画面に出す', async () => {
    getStatus.mockResolvedValue(status())
    renderPage()

    expect(await screen.findByText('実行中（正常）')).toBeInTheDocument()
    expect(screen.getByText('world')).toBeInTheDocument()
    // 設定バージョンとワールドのバージョンの両方に出る
    expect(screen.getAllByText('26.2')).toHaveLength(2)
    expect(screen.getByText('0 / 5')).toBeInTheDocument()
    expect(screen.getByText('有効')).toBeInTheDocument()
  })

  // EDGE-002: 出力を解釈できないと -1 が返る。そのまま出さない。
  it('人数が不明なら不明と出す', async () => {
    getStatus.mockResolvedValue(status({ onlinePlayers: -1 }))
    renderPage()

    expect(await screen.findByText('不明')).toBeInTheDocument()
  })

  it('実行中でもヘルスチェックに通っていなければ区別する', async () => {
    getStatus.mockResolvedValue(status({ healthy: false }))
    renderPage()

    expect(await screen.findByText('実行中')).toBeInTheDocument()
  })

  it('状態を取れないときは理由を出す', async () => {
    getStatus.mockRejectedValue(new ConnectError('サーバーに接続できません', Code.Unavailable))
    renderPage()

    expect(await screen.findByText(/接続できません/)).toBeInTheDocument()
  })
})

describe('操作', () => {
  it('停止中は起動だけ押せる', async () => {
    getStatus.mockResolvedValue(status({ containerState: ContainerState.EXITED, healthy: false }))
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: '起動' })).toBeEnabled())
    expect(screen.getByRole('button', { name: '停止' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '再起動' })).toBeDisabled()
  })

  it('実行中は停止と再起動を押せる', async () => {
    getStatus.mockResolvedValue(status())
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: '停止' })).toBeEnabled())
    expect(screen.getByRole('button', { name: '起動' })).toBeDisabled()
  })

  it('押すと操作が始まる', async () => {
    getStatus.mockResolvedValue(status())
    restartServer.mockResolvedValue({ operation: op() })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '再起動' }))
    await waitFor(() => expect(restartServer).toHaveBeenCalled())
  })

  /**
   * REQ-201: 実行中は変更を伴う操作を無効化する。
   * サーバー側の排他を画面に映しておかないと、押しても
   * failed_precondition が返るだけで理由が分からない。
   */
  it('操作の実行中はすべて押せない', async () => {
    getStatus.mockResolvedValue(status())
    const running = op({ state: OperationState.RUNNING, stepIndex: 1 })
    renderPage({
      active: async () => running,
      watch: () => ({
        async *[Symbol.asyncIterator]() {
          yield create(WatchOperationResponseSchema, { seq: 1n, message: '停止しています', snapshot: running })
          await new Promise(() => {})
        },
      }),
    })

    await waitFor(() => {
      expect(screen.getByRole('button', { name: '停止' })).toBeDisabled()
      expect(screen.getByRole('button', { name: '再起動' })).toBeDisabled()
      expect(screen.getByRole('button', { name: '起動' })).toBeDisabled()
    })
  })

  it('失敗したら理由を出す', async () => {
    getStatus.mockResolvedValue(status())
    restartServer.mockRejectedValue(new ConnectError('他の操作が実行中です', Code.FailedPrecondition))
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '再起動' }))
    expect(await screen.findByText('他の操作が実行中です')).toBeInTheDocument()
  })
})

describe('人がいるときの停止', () => {
  it('確認してからでないと止めない', async () => {
    getStatus.mockResolvedValue(status({ onlinePlayers: 2 }))
    stopServer.mockResolvedValue({ operation: op() })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '停止' }))
    expect(stopServer).not.toHaveBeenCalled()
    expect(screen.getByText(/2 人が接続しています/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '停止する' }))
    await waitFor(() => expect(stopServer).toHaveBeenCalled())
  })

  it('やめると何も起きない', async () => {
    getStatus.mockResolvedValue(status({ onlinePlayers: 2 }))
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '停止' }))
    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))

    expect(stopServer).not.toHaveBeenCalled()
    expect(screen.queryByText(/接続しています/)).not.toBeInTheDocument()
  })

  // 誰もいなければ確認は挟まない。毎回二度押しは煩わしい。
  it('誰もいなければそのまま止める', async () => {
    getStatus.mockResolvedValue(status({ onlinePlayers: 0 }))
    stopServer.mockResolvedValue({ operation: op() })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '停止' }))
    await waitFor(() => expect(stopServer).toHaveBeenCalled())
  })

  // 人数が不明（-1）のときに確認を出すと、毎回二度押しになる。
  it('人数が不明ならそのまま止める', async () => {
    getStatus.mockResolvedValue(status({ onlinePlayers: -1 }))
    stopServer.mockResolvedValue({ operation: op() })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '停止' }))
    await waitFor(() => expect(stopServer).toHaveBeenCalled())
  })
})
