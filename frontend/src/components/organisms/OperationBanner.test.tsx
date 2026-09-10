import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import type { OperationSource } from '../../features/operations'
import { LogLevel, OperationKind, OperationState } from '../../gen/mcadmin/v1/common_pb'
import { OperationSchema, WatchOperationResponseSchema } from '../../gen/mcadmin/v1/operation_pb'
import type { Operation } from '../../gen/mcadmin/v1/operation_pb'
import { withProviders } from '../../test/providers'
import { OperationBanner } from './OperationBanner'

function op(overrides: MessageInitShape<typeof OperationSchema> = {}): Operation {
  return create(OperationSchema, {
    id: 'op-1',
    kind: OperationKind.BACKUP_RESTORE,
    state: OperationState.RUNNING,
    stepIndex: 3,
    stepTotal: 7,
    currentStep: 'アーカイブを展開しています',
    ...overrides,
  })
}

function sourceOf(active: Operation | null, events: ReturnType<typeof line>[]): OperationSource {
  let remaining = active
  return {
    // サーバーは終端に達した操作を「進行中」として返さない。
    // 一度渡したら以降は無いものとして振る舞わせる。
    active: async () => {
      const current = remaining
      remaining = null
      return current
    },
    watch: () => ({
      async *[Symbol.asyncIterator]() {
        for (const e of events) {
          yield e
        }
      },
    }),
  }
}

function line(seq: number, message: string, level: LogLevel, snapshot: Operation) {
  return create(WatchOperationResponseSchema, {
    seq: BigInt(seq),
    level,
    message,
    snapshot,
  })
}

function renderBanner(source: OperationSource) {
  return render(withProviders(<OperationBanner />, source))
}

describe('OperationBanner', () => {
  it('操作の進捗として識別できる領域を持つ', async () => {
    const running = op()
    renderBanner(sourceOf(running, [line(1, '展開しています', LogLevel.INFO, running)]))

    // スピナーと入れ子にならないこと。入れ子のライブリージョンは
    // 同じ進捗を二重に読み上げる。
    expect(await screen.findByRole('status', { name: '操作の進捗' })).toBeInTheDocument()
    expect(screen.queryByRole('status', { name: '処理中' })).not.toBeInTheDocument()
  })

  it('操作が無ければ何も出さない', () => {
    const { container } = renderBanner(sourceOf(null, []))
    expect(container).toBeEmptyDOMElement()
  })

  it('進行中は種類・状態・ステップを示す', async () => {
    const running = op()
    renderBanner(sourceOf(running, [line(1, '展開しています', LogLevel.INFO, running)]))

    expect(await screen.findByText('バックアップからの復元')).toBeInTheDocument()
    expect(screen.getByText('実行中')).toBeInTheDocument()
    expect(screen.getByText(/3\/7 アーカイブを展開しています/)).toBeInTheDocument()
  })

  it('ログは畳んであり、開くと全文が見える', async () => {
    const running = op()
    renderBanner(
      sourceOf(running, [
        line(1, '停止しました', LogLevel.INFO, running),
        line(2, '退避しました', LogLevel.INFO, running),
      ]),
    )

    expect(await screen.findByRole('button', { name: 'ログ (2)' })).toBeInTheDocument()
    expect(screen.queryByText('停止しました')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'ログ (2)' }))
    expect(screen.getByText('停止しました')).toBeInTheDocument()
    expect(screen.getByText('退避しました')).toBeInTheDocument()
  })

  /**
   * 失敗の理由を伝える場所はここしかない。終端で自動的に消すと
   * 「何も言わずに操作が消えた」ことになる。
   */
  it('失敗したら理由を出し、閉じるまで残る', async () => {
    const failed = op({ state: OperationState.FAILED, errorMessage: '空き容量が足りません' })
    renderBanner(sourceOf(op(), [line(1, 'だめでした', LogLevel.ERROR, failed)]))

    expect(await screen.findByText('空き容量が足りません')).toBeInTheDocument()
    expect(screen.getByText('失敗')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '閉じる' }))
    expect(screen.queryByText('空き容量が足りません')).not.toBeInTheDocument()
  })

  // 進行中に閉じられると、進捗を追う手段が無くなる。
  it('進行中は閉じられない', async () => {
    const running = op()
    renderBanner(sourceOf(running, [line(1, '展開しています', LogLevel.INFO, running)]))

    await screen.findByText('実行中')
    expect(screen.queryByRole('button', { name: '閉じる' })).not.toBeInTheDocument()
  })

  it('成功したら完了として残る', async () => {
    const done = op({ state: OperationState.SUCCEEDED, stepIndex: 7 })
    renderBanner(sourceOf(op(), [line(1, '完了しました', LogLevel.INFO, done)]))

    expect(await screen.findByText('完了')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '閉じる' })).toBeInTheDocument()
  })

  /**
   * バイト数が取れるときはそちらで進みを示す。ステップ単位だけだと、
   * 30MB の展開中に帯が何十秒も止まって見える。
   */
  it('バイト数が分かるときはそれで進みを示す', async () => {
    const running = op({ bytesDone: 3_000_000n, bytesTotal: 12_000_000n })
    renderBanner(sourceOf(running, [line(1, '展開しています', LogLevel.INFO, running)]))

    const bar = await screen.findByRole('progressbar', { name: '処理の進み具合' })
    expect(bar).toHaveAttribute('aria-valuenow', '3000000')
    expect(bar).toHaveAttribute('aria-valuemax', '12000000')
  })

  it('バイト数が無ければ手順で進みを示す', async () => {
    const running = op()
    renderBanner(sourceOf(running, [line(1, '展開しています', LogLevel.INFO, running)]))

    const bar = await screen.findByRole('progressbar', { name: '手順の進み具合' })
    expect(bar).toHaveAttribute('aria-valuenow', '3')
  })
})
