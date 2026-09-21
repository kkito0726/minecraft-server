import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { GetMetricsResponseSchema } from '../gen/mcadmin/v1/system_pb'
import { withProviders } from '../test/providers'
import { SystemPage } from './SystemPage'

const getMetrics = vi.hoisted(() => vi.fn())

vi.mock('../features/system/client', () => ({
  systemClient: { getMetrics },
}))

afterEach(() => {
  vi.clearAllMocks()
})

type Overrides = Extract<
  MessageInitShape<typeof GetMetricsResponseSchema>,
  { $typeName?: never }
>

function metrics(overrides: Overrides = {}) {
  return create(GetMetricsResponseSchema, {
    cpu: {
      available: true,
      usedPercent: 28.4,
      windowSeconds: 5,
      cores: 4,
      load1: 1.2,
      load5: 0.9,
      load15: 0.7,
    },
    memory: {
      available: true,
      totalBytes: 4n * 1024n ** 3n,
      availableBytes: 1n * 1024n ** 3n,
      usedBytes: 3n * 1024n ** 3n,
      usedPercent: 75,
    },
    storage: {
      available: true,
      path: '/home/pi/minecraft-server',
      totalBytes: 100n * 1024n ** 3n,
      availableBytes: 60n * 1024n ** 3n,
      usedBytes: 40n * 1024n ** 3n,
      usedPercent: 40,
    },
    ...overrides,
  })
}

describe('リソースの表示', () => {
  it('CPU・メモリ・ストレージを 1 画面に出す', async () => {
    getMetrics.mockResolvedValue(metrics())
    render(withProviders(<SystemPage />))

    expect(await screen.findByRole('meter', { name: 'CPU の使用率' })).toHaveAttribute(
      'aria-valuenow',
      '28',
    )
    expect(screen.getByRole('meter', { name: 'メモリ の使用率' })).toHaveAttribute(
      'aria-valuenow',
      '75',
    )
    expect(screen.getByRole('meter', { name: 'ストレージ の使用率' })).toHaveAttribute(
      'aria-valuenow',
      '40',
    )
    expect(screen.getByText('/home/pi/minecraft-server')).toBeInTheDocument()
  })

  // どの期間の平均かを隠さない。瞬間値だと思って判断されないため。
  it('CPU は測定区間を添える', async () => {
    getMetrics.mockResolvedValue(metrics())
    render(withProviders(<SystemPage />))

    expect(await screen.findAllByText('直近 5 秒の平均')).not.toHaveLength(0)
    expect(screen.getByText('1.20 / 0.90 / 0.70（4 コア）')).toBeInTheDocument()
  })

  // 1 項目が読めないだけで画面を空にしない。
  // 資源が苦しいときにこそ開く画面で、何も見えないのが一番困る。
  it('読めなかった項目は理由を出し、他の項目は表示を続ける', async () => {
    getMetrics.mockResolvedValue(
      metrics({
        cpu: {
          available: false,
          cores: 4,
          unavailableReason: '測定中です（次の取得から出ます）',
        },
      }),
    )
    render(withProviders(<SystemPage />))

    expect(await screen.findByText('測定中です（次の取得から出ます）')).toBeInTheDocument()
    expect(screen.queryByRole('meter', { name: 'CPU の使用率' })).not.toBeInTheDocument()
    // メモリとストレージは出たまま。
    expect(screen.getByRole('meter', { name: 'メモリ の使用率' })).toBeInTheDocument()
    expect(screen.getByRole('meter', { name: 'ストレージ の使用率' })).toBeInTheDocument()
  })

  // スワップは無効にしてある前提（docs/raspberry-pi.md）。
  // 有効になっていること自体が気づきたい事実なので、0 でも黙らせない。
  it('スワップが無いことを明示する', async () => {
    getMetrics.mockResolvedValue(metrics())
    render(withProviders(<SystemPage />))

    expect(await screen.findByText('無効')).toBeInTheDocument()
  })
})
