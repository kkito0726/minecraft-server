import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { OperationKind, OperationState } from '../gen/mcadmin/v1/common_pb'
import { OperationSchema } from '../gen/mcadmin/v1/operation_pb'
import { ListWorldsResponseSchema } from '../gen/mcadmin/v1/world_pb'
import type { OperationSource } from '../features/operations'
import { withProviders } from '../test/providers'
import { WorldsPage } from './WorldsPage'

const listWorlds = vi.hoisted(() => vi.fn())
const switchWorld = vi.hoisted(() => vi.fn())
const createWorld = vi.hoisted(() => vi.fn())
const cloneWorld = vi.hoisted(() => vi.fn())
const renameWorld = vi.hoisted(() => vi.fn())
const deleteWorld = vi.hoisted(() => vi.fn())
const purgeQuarantine = vi.hoisted(() => vi.fn())

vi.mock('../features/worlds/client', () => ({
  worldClient: {
    listWorlds,
    switchWorld,
    createWorld,
    cloneWorld,
    renameWorld,
    deleteWorld,
    purgeQuarantine,
  },
}))

afterEach(() => {
  vi.clearAllMocks()
})

type ListOverrides = Extract<
  MessageInitShape<typeof ListWorldsResponseSchema>,
  { $typeName?: never }
>

function worlds(overrides: ListOverrides = {}) {
  return create(ListWorldsResponseSchema, {
    activeLevel: 'world',
    worlds: [
      {
        name: 'world',
        active: true,
        sizeBytes: 26_420_788n,
        version: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
      },
      {
        name: 'creative',
        active: false,
        sizeBytes: 1_048_576n,
        version: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'creative' },
      },
    ],
    ...overrides,
  })
}

const op = create(OperationSchema, {
  id: 'op-1',
  kind: OperationKind.WORLD_SWITCH,
  state: OperationState.PENDING,
  stepTotal: 5,
})

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
  return render(withProviders(<WorldsPage />, source))
}

describe('一覧', () => {
  it('ワールドと稼働中の印を出す', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    expect(await screen.findByText('world')).toBeInTheDocument()
    expect(screen.getByText('creative')).toBeInTheDocument()
    expect(screen.getByText('稼働中')).toBeInTheDocument()
    expect(screen.getByText('25.2 MB')).toBeInTheDocument()
  })

  // 稼働中のワールドへは切り替えられない。切替はコンテナの再作成を伴う。
  it('稼働中のワールドに切替ボタンを出さない', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    await screen.findByText('world')
    expect(screen.getAllByRole('button', { name: '切替' })).toHaveLength(1)
  })

  // 稼働中のワールドは削除できない。先に切り替えてもらう。
  it('稼働中のワールドは削除できない', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    await screen.findByText('world')
    const [活動中の削除] = screen.getAllByRole('button', { name: '削除' })
    expect(活動中の削除).toBeDisabled()
  })

  it('読めないときは理由を出す', async () => {
    listWorlds.mockRejectedValue(new ConnectError('サーバーに接続できません', Code.Unavailable))
    renderPage()

    expect(await screen.findByText(/接続できません/)).toBeInTheDocument()
  })

  it('1 つも無ければその旨を出す', async () => {
    listWorlds.mockResolvedValue(worlds({ worlds: [] }))
    renderPage()

    expect(await screen.findByText('ワールドがありません。')).toBeInTheDocument()
  })
})

describe('切替', () => {
  it('押すと切り替わる', async () => {
    listWorlds.mockResolvedValue(worlds())
    switchWorld.mockResolvedValue({ operation: op })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '切替' }))
    await waitFor(() => expect(switchWorld).toHaveBeenCalledWith({ name: 'creative' }))
  })
})

describe('新規作成', () => {
  it('名前の規則を満たすまで実行できない', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '新規作成' }))
    expect(screen.getByRole('button', { name: '作成する' })).toBeDisabled()

    await userEvent.type(screen.getByLabelText('ワールド名'), 'ワールド')
    expect(screen.getByRole('button', { name: '作成する' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('英数字')

    await userEvent.clear(screen.getByLabelText('ワールド名'))
    await userEvent.type(screen.getByLabelText('ワールド名'), 'newworld')
    expect(screen.getByRole('button', { name: '作成する' })).toBeEnabled()
  })

  it('シードを添えて作成できる', async () => {
    listWorlds.mockResolvedValue(worlds())
    createWorld.mockResolvedValue({ operation: op })
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '新規作成' }))
    await userEvent.type(screen.getByLabelText('ワールド名'), 'newworld')
    await userEvent.type(screen.getByLabelText(/シード/), '12345')
    await userEvent.click(screen.getByRole('button', { name: '作成する' }))

    await waitFor(() =>
      expect(createWorld).toHaveBeenCalledWith({ name: 'newworld', seed: '12345' }),
    )
  })

  // EDGE-102: data/ の既存ディレクトリと衝突する名前は使えない。
  it('予約語は受け付けない', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    await userEvent.click(await screen.findByRole('button', { name: '新規作成' }))
    await userEvent.type(screen.getByLabelText('ワールド名'), 'plugins')

    expect(screen.getByRole('button', { name: '作成する' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('既存のディレクトリ')
  })
})

describe('複製と改名', () => {
  it('複製元を引き継いで複製する', async () => {
    listWorlds.mockResolvedValue(worlds())
    cloneWorld.mockResolvedValue({ operation: op })
    renderPage()

    await screen.findByText('world')
    await userEvent.click(screen.getAllByRole('button', { name: '複製' })[0]!)
    await userEvent.type(screen.getByLabelText('ワールド名'), 'copy')
    await userEvent.click(screen.getByRole('button', { name: '複製する' }))

    await waitFor(() =>
      expect(cloneWorld).toHaveBeenCalledWith({ source: 'world', destination: 'copy' }),
    )
  })

  it('改名元を引き継いで改名する', async () => {
    listWorlds.mockResolvedValue(worlds())
    renameWorld.mockResolvedValue({ operation: op })
    renderPage()

    await screen.findByText('world')
    await userEvent.click(screen.getAllByRole('button', { name: '改名' })[0]!)
    await userEvent.type(screen.getByLabelText('ワールド名'), 'renamed')
    await userEvent.click(screen.getByRole('button', { name: '改名する' }))

    await waitFor(() => expect(renameWorld).toHaveBeenCalledWith({ from: 'world', to: 'renamed' }))
  })
})

describe('削除', () => {
  /**
   * REQ-123。名前の完全一致を要求する。バックエンドも同じ検証をするが、
   * 気づかせるために画面でも止める。
   */
  it('名前を打ち込まないと削除しない', async () => {
    listWorlds.mockResolvedValue(worlds())
    deleteWorld.mockResolvedValue({ operation: op })
    renderPage()

    await screen.findByText('creative')
    await userEvent.click(screen.getAllByRole('button', { name: '削除' })[1]!)

    expect(screen.getByRole('button', { name: '削除する' })).toBeDisabled()
    await userEvent.type(screen.getByRole('textbox'), 'creative')
    await userEvent.click(screen.getByRole('button', { name: '削除する' }))

    await waitFor(() =>
      expect(deleteWorld).toHaveBeenCalledWith({
        name: 'creative',
        confirmName: 'creative',
        // 既定は退避。復元と同じく、まず mv してから人が消す。
        quarantineInstead: true,
      }),
    )
  })
})

describe('退避したワールド', () => {
  const withQuarantine = () =>
    worlds({
      quarantines: [
        {
          name: 'world.broken-20260910-160815',
          originalLevel: 'world',
          sizeBytes: 26_420_841n,
          fromRestore: true,
        },
      ],
    })

  /**
   * REQ-116。自動では消さない。ここを自動化すると、復元がうまく
   * いかなかったときに戻す先が無くなる。
   */
  it('一覧に出して、消すかどうかは人に委ねる', async () => {
    listWorlds.mockResolvedValue(withQuarantine())
    renderPage()

    expect(await screen.findByText('world.broken-20260910-160815')).toBeInTheDocument()
    expect(screen.getByText('復元による退避')).toBeInTheDocument()
    expect(screen.getByText(/確認してから削除/)).toBeInTheDocument()
  })

  it('完全に削除できる', async () => {
    listWorlds.mockResolvedValue(withQuarantine())
    purgeQuarantine.mockResolvedValue({ freedBytes: 26_420_841n })
    renderPage()

    await screen.findByText('world.broken-20260910-160815')
    await userEvent.click(screen.getByRole('button', { name: '完全に削除' }))

    await waitFor(() =>
      expect(purgeQuarantine).toHaveBeenCalledWith({ name: 'world.broken-20260910-160815' }),
    )
  })

  it('無ければ節ごと出さない', async () => {
    listWorlds.mockResolvedValue(worlds())
    renderPage()

    await screen.findByText('world')
    expect(screen.queryByText('退避したワールド')).not.toBeInTheDocument()
  })
})
