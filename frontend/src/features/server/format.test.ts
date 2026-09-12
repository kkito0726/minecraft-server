import { describe, expect, it } from 'vitest'

import { ContainerState, SavingState } from '../../gen/mcadmin/v1/server_pb'
import {
  containerStateLabel,
  playerCountLabel,
  savingStateLabel,
  serverTone,
  uptimeLabel,
} from './format'

describe('playerCountLabel', () => {
  /**
   * rcon-cli list の出力書式はサーバーの版に依存する。解釈できない場合
   * バックエンドは -1 を返す。これをそのまま出すと「-1 人」になる。
   */
  it('人数が不明なら不明と出す', () => {
    expect(playerCountLabel(-1, 5)).toBe('不明')
  })

  it.each([
    [0, 5, '0 / 5'],
    [3, 5, '3 / 5'],
    [5, 5, '5 / 5'],
  ])('%s / %s → %s', (online, max, want) => {
    expect(playerCountLabel(online, max)).toBe(want)
  })

  it('最大人数が不明でも人数は出す', () => {
    expect(playerCountLabel(2, 0)).toBe('2')
  })
})

describe('containerStateLabel', () => {
  it.each([
    [ContainerState.RUNNING, false, '実行中'],
    [ContainerState.RUNNING, true, '実行中（正常）'],
    [ContainerState.EXITED, false, '停止'],
    [ContainerState.MISSING, false, 'コンテナなし'],
    [ContainerState.RESTARTING, false, '再起動中'],
    [ContainerState.UNSPECIFIED, false, '不明'],
  ])('%s healthy=%s → %s', (state, healthy, want) => {
    expect(containerStateLabel(state, healthy)).toBe(want)
  })
})

describe('serverTone', () => {
  // 実行中でもヘルスチェック前は遊べない。正常と同じ色にすると見分けられない。
  it.each([
    [ContainerState.RUNNING, true, 'ok'],
    [ContainerState.RUNNING, false, 'warn'],
    [ContainerState.RESTARTING, false, 'warn'],
    [ContainerState.EXITED, false, 'off'],
    [ContainerState.MISSING, false, 'off'],
    [ContainerState.UNSPECIFIED, false, 'off'],
  ])('%s healthy=%s → %s', (state, healthy, want) => {
    expect(serverTone(state, healthy)).toBe(want)
  })
})

describe('savingStateLabel', () => {
  // SUSPECT_OFF は「save-off が残っているかもしれない」という推定。
  // 断定できないことが伝わる文言にする。
  it.each([
    [SavingState.ASSUMED_ON, '有効'],
    [SavingState.SUSPECT_OFF, '停止したままの可能性'],
    [SavingState.UNSPECIFIED, '不明'],
  ])('%s → %s', (state, want) => {
    expect(savingStateLabel(state)).toBe(want)
  })
})

describe('uptimeLabel', () => {
  const now = new Date('2026-09-10T12:00:00Z')

  it.each([
    ['45 秒', new Date('2026-09-10T11:59:15Z'), '45 秒'],
    ['5 分', new Date('2026-09-10T11:55:00Z'), '5 分'],
    ['2 時間', new Date('2026-09-10T10:00:00Z'), '2 時間 0 分'],
    ['3 日', new Date('2026-09-07T12:00:00Z'), '3 日 0 時間'],
  ])('%s', (_name, started, want) => {
    expect(uptimeLabel(started, now)).toBe(want)
  })

  it('起動時刻が無ければ空', () => {
    expect(uptimeLabel(undefined, now)).toBe('')
  })

  // 時計のずれで未来になることがある。マイナスを出さない。
  it('未来の時刻でも壊れない', () => {
    expect(uptimeLabel(new Date('2026-09-10T12:00:30Z'), now)).toBe('0 秒')
  })
})
