import { z } from 'zod'

import { Difficulty } from '../../gen/mcadmin/v1/server_pb'
import type { GameSettings } from '../../gen/mcadmin/v1/server_pb'

/**
 * ゲーム設定の入力検証。
 *
 * **範囲はバックエンドの domain/settings と同じ数字でなければならない。**
 * 片方だけ緩むと、画面で通った値をサーバーが弾き、理由が往復してから届く。
 * 一致は settings.test.ts が settings.go を読んで確かめる。
 */
export const SETTINGS_LIMITS = {
  minMaxPlayers: 1,
  maxMaxPlayers: 100,
  minDistance: 3,
  maxDistance: 32,
  maxMotdLength: 100,
} as const

/**
 * Pi 5 (4GB) での目安。超えても保存はできるが、重くなることを先に伝える。
 *
 * 根拠は .env.example のコメント。距離の設定がプレイヤー 1 人あたりの
 * CPU を最も左右し、simulation は view より重い。
 */
export const PI_RECOMMENDED = {
  maxPlayers: 10,
  viewDistance: 10,
  simulationDistance: 8,
} as const

/** 入力欄の値。数値の欄は打ちかけを扱うため文字列で持つ。 */
export type GameSettingsForm = {
  difficulty: Difficulty
  motd: string
  maxPlayers: string
  viewDistance: string
  simulationDistance: string
}

/** 検証を通った値。サーバーへ送る形。 */
export type GameSettingsValues = {
  difficulty: Difficulty
  motd: string
  maxPlayers: number
  viewDistance: number
  simulationDistance: number
}

export type GameSettingsResult =
  | { ok: true; value: GameSettingsValues }
  | { ok: false; message: string }

const { minMaxPlayers, maxMaxPlayers, minDistance, maxDistance, maxMotdLength } = SETTINGS_LIMITS

function rangedInt(label: string, min: number, max: number) {
  const range = `${label}は ${min}〜${max} にしてください`
  return z.coerce
    .number({ message: `${label}は数字で入力してください` })
    .int(`${label}は整数で入力してください`)
    .min(min, range)
    .max(max, range)
}

const numbersSchema = z.object({
  maxPlayers: rangedInt('最大人数', minMaxPlayers, maxMaxPlayers),
  viewDistance: rangedInt('描画距離', minDistance, maxDistance),
  simulationDistance: rangedInt('シミュレーション距離', minDistance, maxDistance),
})

/**
 * MOTD に使えない文字。
 *
 * .env は docker compose がシェルに近い規則で読む。二重引用符の中でも $ は
 * 変数展開され、` や \ の扱いは実装ごとに揺れる。バックエンドと同じく弾く。
 */
const FORBIDDEN_MOTD_CHARS = ['"', '$', '`', '\\']

/**
 * 制御文字を含むか。
 *
 * 正規表現に制御文字の範囲を書くと、見えない文字が混ざって読み違えやすい。
 * 文字コードで判定する。範囲は Go の unicode.IsControl と同じ C0 と C1。
 */
function hasControlChar(value: string): boolean {
  return Array.from(value).some((ch) => {
    const code = ch.codePointAt(0) ?? 0
    return code < 0x20 || (code >= 0x7f && code <= 0x9f)
  })
}

/** MOTD の問題を返す。問題なければ undefined。 */
export function motdProblem(motd: string): string | undefined {
  if (motd.trim() === '') {
    // 空にすると compose の既定値に置き換わり、書いた内容と表示が食い違う。
    return 'MOTD は空にできません'
  }
  // 文字数は見た目の 1 文字で数える。length は UTF-16 の単位なので絵文字で倍になる。
  if (Array.from(motd).length > maxMotdLength) {
    return `MOTD は ${maxMotdLength} 文字以内にしてください`
  }
  if (hasControlChar(motd)) {
    return 'MOTD に改行や制御文字は使えません'
  }
  if (FORBIDDEN_MOTD_CHARS.some((ch) => motd.includes(ch))) {
    return 'MOTD に " $ ` \\ は使えません'
  }
  return undefined
}

export function parseGameSettings(form: GameSettingsForm): GameSettingsResult {
  if (form.difficulty === Difficulty.UNSPECIFIED) {
    return { ok: false, message: '難易度を選んでください' }
  }

  const motdError = motdProblem(form.motd)
  if (motdError) {
    return { ok: false, message: motdError }
  }

  // z.coerce は空文字を 0 に変える。打ちかけの空欄を 0 として
  // 範囲外の理由を出すと、何を直せばよいかが伝わらない。
  if ([form.maxPlayers, form.viewDistance, form.simulationDistance].some((v) => v.trim() === '')) {
    return { ok: false, message: '最大人数と距離を入力してください' }
  }

  const parsed = numbersSchema.safeParse(form)
  if (!parsed.success) {
    return { ok: false, message: parsed.error.issues[0]?.message ?? '入力が正しくありません' }
  }

  const { maxPlayers, viewDistance, simulationDistance } = parsed.data
  // 描画されない範囲まで処理しても、見えない場所の CPU を使うだけになる。
  if (simulationDistance > viewDistance) {
    return {
      ok: false,
      message: `シミュレーション距離（${simulationDistance}）は描画距離（${viewDistance}）以下にしてください`,
    }
  }

  return {
    ok: true,
    value: { difficulty: form.difficulty, motd: form.motd, maxPlayers, viewDistance, simulationDistance },
  }
}

/** 入力の理由を返す。妥当なら undefined。 */
export function gameSettingsError(form: GameSettingsForm): string | undefined {
  const result = parseGameSettings(form)
  return result.ok ? undefined : result.message
}

/**
 * Pi での目安を超えている項目の注意。
 *
 * 規則違反ではないので保存は止めない。重くなることを押す前に知らせるだけ。
 */
export function piLoadNotes(values: GameSettingsValues): string[] {
  const notes: string[] = []
  if (values.maxPlayers > PI_RECOMMENDED.maxPlayers) {
    notes.push(
      `最大人数が ${PI_RECOMMENDED.maxPlayers} 人を超えています。Pi 5 (4GB) では処理が追いつかないことがあります`,
    )
  }
  if (values.viewDistance > PI_RECOMMENDED.viewDistance) {
    notes.push(`描画距離が ${PI_RECOMMENDED.viewDistance} を超えています。帯域と CPU を大きく使います`)
  }
  if (values.simulationDistance > PI_RECOMMENDED.simulationDistance) {
    notes.push(
      `シミュレーション距離が ${PI_RECOMMENDED.simulationDistance} を超えています。巻き戻りの原因になりやすい設定です`,
    )
  }
  return notes
}

/** サーバーから届いた設定を画面の値にする。 */
export function valuesFromProto(settings: GameSettings): GameSettingsValues {
  return {
    difficulty: settings.difficulty,
    motd: settings.motd,
    maxPlayers: settings.maxPlayers,
    viewDistance: settings.viewDistance,
    simulationDistance: settings.simulationDistance,
  }
}

/** 画面の値を入力欄の形にする。 */
export function toForm(values: GameSettingsValues): GameSettingsForm {
  return {
    difficulty: values.difficulty,
    motd: values.motd,
    maxPlayers: String(values.maxPlayers),
    viewDistance: String(values.viewDistance),
    simulationDistance: String(values.simulationDistance),
  }
}

/** 入力欄が元の値から変わっているか。保存ボタンの活性判定に使う。 */
export function isChanged(form: GameSettingsForm, original: GameSettingsForm): boolean {
  return (Object.keys(form) as (keyof GameSettingsForm)[]).some((key) => form[key] !== original[key])
}
