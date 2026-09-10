import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ContainerState, GetStatusResponseSchema } from '../../gen/mcadmin/v1/server_pb'
import { InterruptedBanner } from './InterruptedBanner'

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
    ...overrides,
  })
}

function renderBanner() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <InterruptedBanner />
    </QueryClientProvider>,
  )
}

describe('InterruptedBanner', () => {
  it('検出されていなければ何も出さない', async () => {
    getStatus.mockResolvedValue(status({ interruptedOperationDetected: false }))
    const { container } = renderBanner()

    await vi.waitFor(() => expect(getStatus).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  /**
   * REQ-203。save-off が残ったままだと以降の変更がディスクに書かれない
   * のに症状が何も出ない。検出できている間は出し続ける。
   */
  it('検出されている間は警告を出し続ける', async () => {
    getStatus.mockResolvedValue(status({ interruptedOperationDetected: true }))
    renderBanner()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('前回の実行が操作の途中で終了した形跡があります')
    expect(alert).toHaveTextContent('save-on を送信し直しました')
  })

  it('状態を取れないうちは何も出さない', () => {
    getStatus.mockReturnValue(new Promise(() => {}))
    const { container } = renderBanner()

    expect(container).toBeEmptyDOMElement()
  })
})
