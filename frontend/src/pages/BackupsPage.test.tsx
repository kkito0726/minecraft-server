import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  BackupMode,
  ListBackupsResponseSchema,
  PreflightRestoreResponseSchema,
  RestoreTarget,
  VersionVerdict,
} from '../gen/mcadmin/v1/backup_pb'
import { OperationKind, OperationState } from '../gen/mcadmin/v1/common_pb'
import { OperationSchema } from '../gen/mcadmin/v1/operation_pb'
import type { OperationSource } from '../features/operations'
import { withProviders } from '../test/providers'
import { BackupsPage } from './BackupsPage'

const listBackups = vi.hoisted(() => vi.fn())
const createBackup = vi.hoisted(() => vi.fn())
const deleteBackup = vi.hoisted(() => vi.fn())
const preflightRestore = vi.hoisted(() => vi.fn())
const restoreBackup = vi.hoisted(() => vi.fn())
const getRetentionPolicy = vi.hoisted(() => vi.fn())
const setRetentionPolicy = vi.hoisted(() => vi.fn())
const pruneBackups = vi.hoisted(() => vi.fn())

vi.mock('../features/backups/client', () => ({
  backupClient: {
    listBackups,
    createBackup,
    deleteBackup,
    preflightRestore,
    restoreBackup,
    getRetentionPolicy,
    setRetentionPolicy,
    pruneBackups,
  },
}))

const BACKUP_ID = 'backup-26.2-world-20260910-160000.zip'

const backups = create(ListBackupsResponseSchema, {
  directory: '/srv/backups',
  totalSizeBytes: 11_400_000n,
  backups: [
    {
      id: BACKUP_ID,
      sizeBytes: 11_400_000n,
      createdAt: { seconds: 1_789_000_000n },
      archiveLevel: 'world',
      version: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
    },
  ],
})

const preflight = create(PreflightRestoreResponseSchema, {
  backup: { id: BACKUP_ID, archiveLevel: 'world' },
  currentLevel: 'world',
  verdict: VersionVerdict.MATCH,
  requiredBytes: 13_680_000n,
  availableBytes: 50_000_000_000n,
})

const op = create(OperationSchema, {
  id: 'op-1',
  kind: OperationKind.BACKUP_RESTORE,
  state: OperationState.PENDING,
  stepTotal: 7,
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

function setup() {
  listBackups.mockResolvedValue(backups)
  getRetentionPolicy.mockResolvedValue({ policy: { keepCount: 10, keepDays: 0 } })
  preflightRestore.mockResolvedValue(preflight)
  createBackup.mockResolvedValue({ operation: op })
  restoreBackup.mockResolvedValue({ operation: op })
  render(withProviders(<BackupsPage />, idleSource))
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('BackupsPage', () => {
  it('一覧を出す', async () => {
    setup()
    expect(await screen.findByText(BACKUP_ID)).toBeInTheDocument()
  })

  it('取得を始める', async () => {
    setup()
    await userEvent.click(await screen.findByRole('button', { name: 'バックアップを取得' }))
    await userEvent.click(screen.getByRole('button', { name: '取得する' }))

    await waitFor(() => {
      expect(createBackup).toHaveBeenCalledWith({ mode: BackupMode.HOT, note: '' })
    })
  })

  /**
   * REQ-008。事前確認はダイアログを開くたびに取り直す。
   * 一覧と一緒にキャッシュすると、古い判定に承諾させることになる。
   */
  it('復元ダイアログを開くたびに事前確認を取り直す', async () => {
    setup()

    await userEvent.click(await screen.findByRole('button', { name: '復元' }))
    await waitFor(() => expect(preflightRestore).toHaveBeenCalledWith({ backupId: BACKUP_ID }))
    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))

    await userEvent.click(screen.getByRole('button', { name: '復元' }))
    await waitFor(() => expect(preflightRestore).toHaveBeenCalledTimes(2))
  })

  it('確認をそろえて復元を始める', async () => {
    setup()

    await userEvent.click(await screen.findByRole('button', { name: '復元' }))
    await userEvent.type(await screen.findByLabelText(/確認のため/), 'world')
    await userEvent.click(screen.getByRole('button', { name: '復元する' }))

    await waitFor(() => {
      expect(restoreBackup).toHaveBeenCalledWith({
        backupId: BACKUP_ID,
        target: RestoreTarget.ARCHIVE_LEVEL,
        acknowledgeVersionWarning: false,
        confirmLevelName: 'world',
      })
    })
  })

  it('保持ポリシーを出す', async () => {
    setup()
    expect(await screen.findByLabelText(/世代/)).toHaveValue('10')
  })

  it('保持ポリシーを保存する', async () => {
    setRetentionPolicy.mockResolvedValue({ policy: { keepCount: 5, keepDays: 0 } })
    setup()

    const count = await screen.findByLabelText(/世代/)
    await userEvent.clear(count)
    await userEvent.type(count, '5')
    await userEvent.click(screen.getByRole('button', { name: '保存する' }))

    await waitFor(() => {
      expect(setRetentionPolicy).toHaveBeenCalledWith({ policy: { keepCount: 5, keepDays: 0 } })
    })
  })

  // dry_run で対象を見せてから消す。何が消えるか分からないまま削除させない。
  it('手動の適用は対象を見せてから消す', async () => {
    pruneBackups.mockResolvedValue({ deletedIds: ['old.zip'], freedBytes: 1n })
    setup()

    await userEvent.click(await screen.findByRole('button', { name: '対象を確認' }))
    expect(await screen.findByText('old.zip')).toBeInTheDocument()
    await waitFor(() => expect(pruneBackups).toHaveBeenCalledWith({ dryRun: true }))

    await userEvent.click(screen.getByRole('button', { name: '削除する' }))
    await waitFor(() => expect(pruneBackups).toHaveBeenCalledWith({ dryRun: false }))

    // 消したあとは予定ではなく実際に消えたものを出す。
    expect(await screen.findByText(/削除しました/)).toBeInTheDocument()
  })

  it('削除は二度押しで消す', async () => {
    deleteBackup.mockResolvedValue({ freedBytes: 11_400_000n })
    setup()

    await userEvent.click(await screen.findByRole('button', { name: '削除' }))
    await userEvent.click(screen.getByRole('button', { name: '本当に削除' }))

    await waitFor(() => expect(deleteBackup).toHaveBeenCalledWith({ backupId: BACKUP_ID }))
  })

  it('一覧が取れなければ理由を出す', async () => {
    listBackups.mockRejectedValue(new ConnectError('バックアップの保管先を読めません', Code.Internal))
    getRetentionPolicy.mockResolvedValue({ policy: { keepCount: 10, keepDays: 0 } })
    render(withProviders(<BackupsPage />, idleSource))

    expect(await screen.findByText(/保管先を読めません/)).toBeInTheDocument()
  })

  it('取得に失敗したら理由を出す', async () => {
    setup()
    createBackup.mockRejectedValue(new ConnectError('他の操作が実行中です', Code.FailedPrecondition))

    await userEvent.click(await screen.findByRole('button', { name: 'バックアップを取得' }))
    await userEvent.click(screen.getByRole('button', { name: '取得する' }))

    expect(await screen.findByText(/他の操作が実行中です/)).toBeInTheDocument()
  })
})
