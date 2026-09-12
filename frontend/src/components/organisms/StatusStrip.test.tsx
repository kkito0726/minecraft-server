import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ContainerState, GetStatusResponseSchema } from '../../gen/mcadmin/v1/server_pb'
import { StatusStrip } from './StatusStrip'

const getStatus = vi.hoisted(() => vi.fn())
vi.mock('../../features/server/client', () => ({ serverClient: { getStatus } }))

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
    onlinePlayers: 2,
    maxPlayers: 5,
    ...overrides,
  })
}

function renderStrip() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <StatusStrip />
    </QueryClientProvider>,
  )
}

describe('StatusStrip', () => {
  it('状態・ワールド・人数・版を要約して出す', async () => {
    getStatus.mockResolvedValue(status())
    renderStrip()

    expect(await screen.findByText('実行中（正常）')).toBeInTheDocument()
    expect(screen.getByText('world')).toBeInTheDocument()
    expect(screen.getByText('2 / 5')).toBeInTheDocument()
    expect(screen.getByText('26.2')).toBeInTheDocument()
  })

  // サーバー画面のカードと見出しが重複すると、見出しでの移動が迷う。
  it('見出しを持たない', async () => {
    getStatus.mockResolvedValue(status())
    renderStrip()

    await screen.findByText('実行中（正常）')
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })

  it('値が空なら未設定・不明と出す', async () => {
    getStatus.mockResolvedValue(
      status({ containerState: ContainerState.EXITED, activeLevel: '', configuredVersion: '' }),
    )
    renderStrip()

    expect(await screen.findByText('停止')).toBeInTheDocument()
    expect(screen.getByText('未設定')).toBeInTheDocument()
    expect(screen.getByText('不明')).toBeInTheDocument()
  })

  it('取得している間はそう伝える', () => {
    getStatus.mockReturnValue(new Promise(() => {}))
    renderStrip()

    expect(screen.getByText('状態を確認しています…')).toBeInTheDocument()
  })

  it('取得できなければそう伝える', async () => {
    getStatus.mockRejectedValue(new ConnectError('down', Code.Unavailable))
    renderStrip()

    expect(await screen.findByText('状態を取得できません')).toBeInTheDocument()
  })
})
