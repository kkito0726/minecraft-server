import { z } from 'zod'

/**
 * ワールド名の正規表現。
 *
 * **バックエンドの `world.NamePattern` と同一の文字列でなければならない**
 * （NFR-304）。片方だけが緩むと、不正な名前がファイルシステムに到達する。
 * 一致は schema.test.ts が backend/internal/domain/world/name.go を
 * 読んで確かめる。
 */
export const WORLD_NAME_PATTERN = '^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$'

/**
 * 使えない名前。data/ 配下に既にあるディレクトリと衝突する（EDGE-102）。
 * バックエンドの reservedNames と同じ内容。
 */
export const RESERVED_WORLD_NAMES = [
  'cache',
  'config',
  'libraries',
  'logs',
  'plugins',
  'versions',
]

const nameRegExp = new RegExp(WORLD_NAME_PATTERN)

export const worldNameSchema = z
  .string()
  .trim()
  .regex(nameRegExp, '英数字で始まる 1〜32 文字（英数字・_・-）にしてください')
  .refine((v) => !RESERVED_WORLD_NAMES.includes(v), {
    message: 'この名前は data/ の既存のディレクトリと重なるため使えません',
  })

/** 名前が使えるかを返す。ボタンの活性判定に使う。 */
export function isValidWorldName(value: string): boolean {
  return worldNameSchema.safeParse(value).success
}

/** 使えない理由を返す。使えるなら空文字。 */
export function worldNameError(value: string): string {
  if (value === '') {
    return ''
  }
  const result = worldNameSchema.safeParse(value)
  return result.success ? '' : (result.error.issues[0]?.message ?? '')
}
