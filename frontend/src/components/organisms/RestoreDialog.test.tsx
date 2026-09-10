import { create } from '@bufbuild/protobuf'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import {
  PreflightRestoreResponseSchema,
  RestoreTarget,
  VersionVerdict,
} from '../../gen/mcadmin/v1/backup_pb'
import { RestoreDialog } from './RestoreDialog'

type Overrides = Extract<
  MessageInitShape<typeof PreflightRestoreResponseSchema>,
  { $typeName?: never }
>

/** 既定は「一致・名前も同じ」= 承諾の要らない最も安全な状態。 */
function preflight(overrides: Overrides = {}) {
  return create(PreflightRestoreResponseSchema, {
    backup: {
      id: 'backup-26.2-world-20260910-160000.zip',
      sizeBytes: 11_400_000n,
      archiveLevel: 'world',
      version: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
    },
    currentLevel: 'world',
    currentWorldVersion: { readable: true, name: '26.2', dataVersion: 4903, levelName: 'world' },
    configuredMcVersion: '26.2',
    verdict: VersionVerdict.MATCH,
    requiresConfirmation: false,
    requiredBytes: 13_680_000n,
    availableBytes: 50_000_000_000n,
    ...overrides,
  })
}

function renderDialog(overrides: Overrides = {}) {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(
    <RestoreDialog preflight={preflight(overrides)} onConfirm={onConfirm} onCancel={onCancel} />,
  )
  return { onConfirm, onCancel }
}

const submit = () => screen.getByRole('button', { name: '復元する' })
const nameInput = () => screen.getByLabelText(/確認のため/)

describe('RestoreDialog', () => {
  /**
   * REQ-123。復元は取り消せない。復元先のワールド名の完全一致を要求する。
   * バックエンドも同じ検証をするが、守りは二重にする。
   */
  it('復元先の名前を打ち込むまで実行できない', async () => {
    renderDialog()

    expect(submit()).toBeDisabled()
    await userEvent.type(nameInput(), 'world')
    expect(submit()).toBeEnabled()
  })

  it('名前が一致しなければ実行できない', async () => {
    renderDialog()

    await userEvent.type(nameInput(), 'worl')
    expect(submit()).toBeDisabled()
  })

  it('一致したら要求を渡す', async () => {
    const { onConfirm } = renderDialog()

    await userEvent.type(nameInput(), 'world')
    await userEvent.click(submit())

    expect(onConfirm).toHaveBeenCalledWith({
      target: RestoreTarget.ARCHIVE_LEVEL,
      acknowledgeVersionWarning: false,
      confirmLevelName: 'world',
    })
  })

  /**
   * REQ-109。バージョンが食い違うときは、承諾のチェックと名前の
   * 両方が揃うまで実行できない。片方だけでは通らない。
   */
  describe('承諾が必要なとき', () => {
    const warned: Overrides = {
      verdict: VersionVerdict.OLDER_WILL_UPGRADE,
      requiresConfirmation: true,
      warnings: ['アーカイブの方が古いバージョンです。復元すると再度アップグレードが走ります。'],
    }

    it('警告文をそのまま出す', () => {
      renderDialog(warned)
      expect(screen.getAllByText(/再度アップグレードが走ります/).length).toBeGreaterThan(0)
    })

    it('名前だけでは実行できない', async () => {
      renderDialog(warned)

      await userEvent.type(nameInput(), 'world')
      expect(submit()).toBeDisabled()
    })

    it('承諾だけでは実行できない', async () => {
      renderDialog(warned)

      await userEvent.click(screen.getByRole('checkbox'))
      expect(submit()).toBeDisabled()
    })

    it('両方そろえば実行できる', async () => {
      const { onConfirm } = renderDialog(warned)

      await userEvent.type(nameInput(), 'world')
      await userEvent.click(screen.getByRole('checkbox'))
      expect(submit()).toBeEnabled()

      await userEvent.click(submit())
      expect(onConfirm).toHaveBeenCalledWith(
        expect.objectContaining({ acknowledgeVersionWarning: true }),
      )
    })
  })

  // 承諾が要らないときにチェックを出すと、常に押す癖がついて意味を失う。
  it('承諾が不要ならチェックを出さない', () => {
    renderDialog()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  /**
   * REQ-112。素直に展開すると data/world が戻るだけで、稼働中の
   * ワールドは何も変わらない。「何も起きていないように見える」罠。
   */
  describe('復元先の選択', () => {
    const mismatch: Overrides = {
      currentLevel: 'creative',
      levelNameMismatch: true,
      requiresConfirmation: true,
      warnings: ['アーカイブのワールド名が現在稼働中のものと違います。'],
    }

    it('名前が食い違うことを両方の名前つきで伝える', () => {
      renderDialog(mismatch)
      expect(screen.getAllByText(/creative/).length).toBeGreaterThan(0)
    })

    it('稼働中の名前で復元すると打ち込む名前が変わる', async () => {
      renderDialog(mismatch)

      await userEvent.click(screen.getByRole('radio', { name: /稼働中/ }))
      await userEvent.type(nameInput(), 'creative')
      await userEvent.click(screen.getByRole('checkbox'))

      expect(submit()).toBeEnabled()
    })

    // 打ち込んだあとに切り替えると、期待される名前が変わる。
    // 入力が残っていると「合っているのに押せない」状態になる。
    it('復元先を変えると打ち込んだ名前を消す', async () => {
      renderDialog(mismatch)

      await userEvent.type(nameInput(), 'world')
      await userEvent.click(screen.getByRole('radio', { name: /稼働中/ }))

      expect(nameInput()).toHaveValue('')
    })

    it('アーカイブのワールド名が読めないときは選べない', () => {
      renderDialog({ backup: { id: 'broken.zip', archiveLevel: '' }, currentLevel: 'world' })
      expect(screen.getByRole('radio', { name: /アーカイブ/ })).toBeDisabled()
    })
  })

  /**
   * 空き容量が足りなければ手順 2 で失敗する。サーバーを止める前に
   * 分かるのだから、押す前に見せる。
   */
  it('空き容量が足りないことを必要量と空き量つきで伝える', () => {
    renderDialog({ requiredBytes: 60_000_000_000n, availableBytes: 1_000_000_000n })

    const notice = screen.getByRole('alert')
    expect(notice).toHaveTextContent('55.9 GB')
    expect(notice).toHaveTextContent('953.7 MB')
  })

  it('空きが足りていれば警告を出さない', () => {
    renderDialog()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  // 復元は「退避してから展開」。元に戻せることを実行前に伝える。
  it('現在のワールドが退避されることを伝える', () => {
    renderDialog()
    expect(screen.getAllByText(/退避/).length).toBeGreaterThan(0)
  })

  it('やめられる', async () => {
    const { onCancel, onConfirm } = renderDialog()

    await userEvent.click(screen.getByRole('button', { name: 'やめる' }))
    expect(onCancel).toHaveBeenCalled()
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('無効化できる', () => {
    render(
      <RestoreDialog preflight={preflight()} disabled onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByRole('button', { name: 'やめる' })).toBeDisabled()
    expect(nameInput()).toBeDisabled()
  })
})
