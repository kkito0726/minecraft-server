import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { LogLevel, OperationKind, OperationState } from '../../gen/mcadmin/v1/common_pb'
import { useOperation } from './context'
import { kindLabel, levelTone, stateLabel } from './labels'

describe('kindLabel', () => {
  // proto の enum をそのまま画面に出さない。英大文字の識別子は
  // 利用者にとって意味を持たない。
  it.each([
    [OperationKind.SERVER_RESTART, 'サーバーの再起動'],
    [OperationKind.BACKUP_CREATE, 'バックアップの取得'],
    [OperationKind.BACKUP_RESTORE, 'バックアップからの復元'],
    [OperationKind.WORLD_SWITCH, 'ワールドの切り替え'],
    [OperationKind.UNSPECIFIED, '操作'],
  ])('%s → %s', (kind, want) => {
    expect(kindLabel(kind)).toBe(want)
  })

  it('知らない種類でも空にしない', () => {
    expect(kindLabel(99 as OperationKind)).toBe('操作')
  })
})

describe('stateLabel', () => {
  it.each([
    [OperationState.PENDING, '待機中'],
    [OperationState.RUNNING, '実行中'],
    [OperationState.SUCCEEDED, '完了'],
    [OperationState.FAILED, '失敗'],
  ])('%s → %s', (state, want) => {
    expect(stateLabel(state)).toBe(want)
  })

  it('知らない状態でも空にしない', () => {
    expect(stateLabel(99 as OperationState)).toBe('不明')
  })
})

describe('levelTone', () => {
  it.each([
    [LogLevel.INFO, 'neutral'],
    [LogLevel.WARN, 'warn'],
    [LogLevel.ERROR, 'danger'],
    [LogLevel.UNSPECIFIED, 'neutral'],
  ])('%s → %s', (level, want) => {
    expect(levelTone(level)).toBe(want)
  })
})

describe('useOperation', () => {
  // Provider の外で使うと、購読していない空の状態を黙って返してしまう。
  // 気づけないバグになるので、はっきり失敗させる。
  it('Provider の外では例外にする', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => renderHook(() => useOperation())).toThrow(/OperationProvider/)
    spy.mockRestore()
  })
})
