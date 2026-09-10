import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ListBackupsResponseSchema } from '../../gen/mcadmin/v1/backup_pb'
import { BackupTable } from './BackupTable'

type Overrides = Extract<
  MessageInitShape<typeof ListBackupsResponseSchema>,
  { $typeName?: never }
>

function list(overrides: Overrides = {}) {
  return create(ListBackupsResponseSchema, {
    directory: '/home/pi/minecraft-server/backups',
    totalSizeBytes: 22_800_000n,
    backups: [
      {
        id: 'backup-26.2-world-20260910-160000.zip',
        sizeBytes: 11_400_000n,
        createdAt: { seconds: 1_789_000_000n },
        declaredVersion: '26.2',
        archiveLevel: 'world',
        version: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
      },
      {
        id: 'backup-26.1-world-20260901-120000.zip',
        sizeBytes: 11_400_000n,
        createdAt: { seconds: 1_788_000_000n },
        declaredVersion: '26.1',
        archiveLevel: 'world',
        version: { readable: false },
      },
    ],
    ...overrides,
  })
}

function renderTable(overrides: Overrides = {}) {
  const onRestore = vi.fn()
  const onDelete = vi.fn()
  render(
    <BackupTable data={list(overrides)} disabled={false} onRestore={onRestore} onDelete={onDelete} />,
  )
  return { onRestore, onDelete }
}

describe('BackupTable', () => {
  it('新しい順にそのまま並べる', () => {
    renderTable()
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows[0]).toHaveTextContent('20260910-160000')
  })

  /**
   * REQ-413。ファイル名の版は改名できるので信用しない。
   * 表示するのはアーカイブ内の level.dat から読んだ値。
   */
  it('読めなかったバージョンは不明と出す', () => {
    renderTable()
    expect(screen.getAllByText('不明').length).toBeGreaterThan(0)
  })

  it('保管先と合計を出す', () => {
    renderTable()
    expect(screen.getByText(/\/home\/pi\/minecraft-server\/backups/)).toBeInTheDocument()
    expect(screen.getByText(/21\.7 MB/)).toBeInTheDocument()
  })

  /**
   * v1 はオフサイト転送を持たない。ディスクが壊れれば
   * バックアップも一緒に失われる。一覧に必ず添える。
   */
  it('このディスク上にしか無いことを必ず伝える', () => {
    renderTable()
    expect(screen.getByText(/このディスク上にしかありません/)).toBeInTheDocument()
  })

  it('復元を選べる', async () => {
    const { onRestore } = renderTable()

    await userEvent.click(screen.getAllByRole('button', { name: '復元' })[0]!)
    expect(onRestore).toHaveBeenCalledWith('backup-26.2-world-20260910-160000.zip')
  })

  // 削除は取り消せない。一度で消えないようにする。
  it('削除は二度押しさせる', async () => {
    const { onDelete } = renderTable()

    await userEvent.click(screen.getAllByRole('button', { name: '削除' })[0]!)
    expect(onDelete).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: '本当に削除' }))
    expect(onDelete).toHaveBeenCalledWith('backup-26.2-world-20260910-160000.zip')
  })

  it('1 つも無いときに案内を出す', () => {
    renderTable({ backups: [], totalSizeBytes: 0n })
    expect(screen.getByText(/まだバックアップがありません/)).toBeInTheDocument()
  })
})
