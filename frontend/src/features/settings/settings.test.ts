import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { Difficulty, GameMode } from '../../gen/mcadmin/v1/server_pb'
import {
  PI_RECOMMENDED,
  SETTINGS_LIMITS,
  gameSettingsError,
  isChanged,
  motdProblem,
  parseGameSettings,
  piLoadNotes,
  toForm,
} from './settings'
import type { GameSettingsForm } from './settings'

const valid: GameSettingsForm = {
  difficulty: Difficulty.HARD,
  mode: GameMode.SURVIVAL,
  motd: '§aようこそ',
  maxPlayers: '8',
  viewDistance: '9',
  simulationDistance: '6',
}

describe('バックエンドとの一致', () => {
  /**
   * 範囲がずれると、画面で通った値をサーバーが弾く。Go のソースを読んで
   * 数字が同じであることを確かめる。ワールド名の正規表現と同じ守り方。
   */
  it('値の範囲が domain/settings と同じ', () => {
    const source = readFileSync(
      join(import.meta.dirname, '../../../../backend/internal/domain/settings/settings.go'),
      'utf8',
    )
    const constant = (name: string) => {
      const match = new RegExp(`${name}\\s*=\\s*(\\d+)`).exec(source)
      if (!match?.[1]) {
        throw new Error(`${name} が settings.go に見つからない`)
      }
      return Number(match[1])
    }

    expect(SETTINGS_LIMITS.minMaxPlayers).toBe(constant('MinMaxPlayers'))
    expect(SETTINGS_LIMITS.maxMaxPlayers).toBe(constant('MaxMaxPlayers'))
    expect(SETTINGS_LIMITS.minDistance).toBe(constant('MinDistance'))
    expect(SETTINGS_LIMITS.maxDistance).toBe(constant('MaxDistance'))
    expect(SETTINGS_LIMITS.maxMotdLength).toBe(constant('MaxMOTDLength'))
  })
})

describe('parseGameSettings', () => {
  it('妥当な値を数値にして返す', () => {
    expect(parseGameSettings(valid)).toEqual({
      ok: true,
      value: {
        difficulty: Difficulty.HARD,
        mode: GameMode.SURVIVAL,
        motd: '§aようこそ',
        maxPlayers: 8,
        viewDistance: 9,
        simulationDistance: 6,
      },
    })
  })

  it.each<[string, Partial<GameSettingsForm>, RegExp]>([
    ['難易度が未選択', { difficulty: Difficulty.UNSPECIFIED }, /難易度/],
    ['モードが未選択', { mode: GameMode.UNSPECIFIED }, /ゲームモード/],
    ['打ちかけの空欄', { maxPlayers: '' }, /入力してください/],
    ['人数が 0', { maxPlayers: '0' }, /最大人数は 1〜100/],
    ['人数が上限超え', { maxPlayers: '101' }, /最大人数は 1〜100/],
    ['人数が小数', { maxPlayers: '2.5' }, /整数/],
    ['人数が数字でない', { maxPlayers: 'many' }, /数字/],
    ['描画距離が小さすぎる', { viewDistance: '2', simulationDistance: '2' }, /描画距離は 3〜32/],
    ['描画距離が大きすぎる', { viewDistance: '33' }, /描画距離は 3〜32/],
    ['シミュレーションが描画より遠い', { viewDistance: '6', simulationDistance: '7' }, /描画距離（6）以下/],
  ])('%s は弾く', (_name, patch, message) => {
    const result = parseGameSettings({ ...valid, ...patch })
    expect(result.ok).toBe(false)
    expect(gameSettingsError({ ...valid, ...patch })).toMatch(message)
  })

  it('範囲の端はちょうど通る', () => {
    expect(parseGameSettings({ ...valid, maxPlayers: '100', viewDistance: '32', simulationDistance: '32' }).ok).toBe(true)
    expect(parseGameSettings({ ...valid, maxPlayers: '1', viewDistance: '3', simulationDistance: '3' }).ok).toBe(true)
  })

  it('妥当なら理由は出ない', () => {
    expect(gameSettingsError(valid)).toBeUndefined()
  })
})

describe('motdProblem', () => {
  it.each([
    ['空', ''],
    ['空白だけ', '   '],
    ['改行', '1 行目\n2 行目'],
    ['二重引用符', 'say "hi"'],
    ['ドル記号（変数展開）', '$HOME'],
    ['バッククォート（コマンド置換）', '`id`'],
    ['バックスラッシュ', 'a\\b'],
    ['上限超え', 'あ'.repeat(SETTINGS_LIMITS.maxMotdLength + 1)],
  ])('%s は弾く', (_name, motd) => {
    expect(motdProblem(motd)).toBeDefined()
  })

  // 文字数は見た目の 1 文字で数える。length だと絵文字が 2 に数えられる。
  it('上限ちょうどは通る。絵文字も 1 文字として数える', () => {
    expect(motdProblem('あ'.repeat(SETTINGS_LIMITS.maxMotdLength))).toBeUndefined()
    expect(motdProblem('🧱'.repeat(SETTINGS_LIMITS.maxMotdLength))).toBeUndefined()
  })

  it('色コードと空白は使える', () => {
    expect(motdProblem('§aE2E のサーバー')).toBeUndefined()
  })

  // Go の unicode.IsControl と同じ範囲を弾く。C1 の制御文字も含む。
  it('C0 と C1 の制御文字は弾く', () => {
    expect(motdProblem(`a${String.fromCharCode(0)}b`)).toBeDefined()
    expect(motdProblem(`a${String.fromCharCode(0x7f)}b`)).toBeDefined()
    expect(motdProblem(`a${String.fromCharCode(0x85)}b`)).toBeDefined()
  })
})

describe('piLoadNotes', () => {
  it('目安の範囲なら何も言わない', () => {
    expect(
      piLoadNotes({
        difficulty: Difficulty.NORMAL,
        mode: GameMode.SURVIVAL,
        motd: 'm',
        maxPlayers: 5,
        viewDistance: 7,
        simulationDistance: 5,
      }),
    ).toEqual([])
  })

  it('目安を超えた項目ごとに注意を出す', () => {
    const notes = piLoadNotes({
      difficulty: Difficulty.NORMAL,
      mode: GameMode.SURVIVAL,
      motd: 'm',
      maxPlayers: PI_RECOMMENDED.maxPlayers + 1,
      viewDistance: PI_RECOMMENDED.viewDistance + 1,
      simulationDistance: PI_RECOMMENDED.simulationDistance + 1,
    })
    expect(notes).toHaveLength(3)
  })
})

describe('toForm / isChanged', () => {
  it('入力欄の形にして、変化を検出する', () => {
    const form = toForm({
      difficulty: Difficulty.EASY,
      mode: GameMode.SURVIVAL,
      motd: 'm',
      maxPlayers: 5,
      viewDistance: 7,
      simulationDistance: 5,
    })
    expect(form.maxPlayers).toBe('5')
    expect(isChanged(form, form)).toBe(false)
    expect(isChanged({ ...form, viewDistance: '8' }, form)).toBe(true)
  })
})
