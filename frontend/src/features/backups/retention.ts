import { z } from 'zod'

/**
 * 保持ポリシーの入力検証。
 *
 * keepCount が 0 だと、次の適用でバックアップが全損する。
 * バックエンドの値オブジェクトが 1 以上を不変条件にしているので
 * （EDGE-104）、画面でも同じ境界で止めて往復を省く。
 */

const countSchema = z.coerce
  .number({ message: '保持世代数は数字で入力してください' })
  .int('保持世代数は整数で入力してください')
  .min(1, '保持世代数は 1 以上にしてください。0 にするとバックアップが全て消えます')

const daysSchema = z.coerce
  .number({ message: '保持日数は数字で入力してください' })
  .int('保持日数は整数で入力してください')
  .min(0, '保持日数は 0 以上にしてください。0 で日数による削除を無効にします')

export type RetentionInput = { keepCount: number; keepDays: number }
export type RetentionResult = ({ ok: true } & RetentionInput) | { ok: false; message: string }

export function parseRetention(keepCount: string, keepDays: string): RetentionResult {
  // z.coerce は空文字を 0 に変える。打ち始めの空欄を「0 世代」と
  // 読むと、指が滑った瞬間に全損の設定が妥当に見えてしまう。
  if (keepCount.trim() === '' || keepDays.trim() === '') {
    return { ok: false, message: '保持世代数と保持日数を入力してください' }
  }

  const parsed = z.object({ keepCount: countSchema, keepDays: daysSchema }).safeParse({
    keepCount,
    keepDays,
  })
  if (!parsed.success) {
    return { ok: false, message: parsed.error.issues[0]?.message ?? '入力が正しくありません' }
  }
  return { ok: true, ...parsed.data }
}

/** 入力の理由を返す。妥当なら undefined。 */
export function retentionError(keepCount: string, keepDays: string): string | undefined {
  const result = parseRetention(keepCount, keepDays)
  return result.ok ? undefined : result.message
}
