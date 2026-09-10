import { z } from 'zod'

/**
 * 管理コンソールの共有トークン。
 *
 * バックエンドは 32 文字未満の ADMIN_TOKEN を起動時に拒否する
 * （`openssl rand -hex 32` で 64 文字を作る想定）。画面も同じ線で断り、
 * 明らかに短い入力をサーバーへ送らない。
 */
export const MIN_TOKEN_LENGTH = 32

const STORAGE_KEY = 'mcadmin.token'

export const tokenSchema = z
  .string()
  .trim()
  .min(MIN_TOKEN_LENGTH, `トークンは ${MIN_TOKEN_LENGTH} 文字以上です`)

export function isValidToken(value: string): boolean {
  return tokenSchema.safeParse(value).success
}

/**
 * 保存済みのトークンを読む。未設定や読み取り不能なら空文字。
 *
 * プライベートウィンドウや「サイトデータをブロック」設定では
 * localStorage の参照そのものが例外を投げる。トークンを保持できない
 * ことと、画面が開けないことは別の問題として扱う。
 */
export function readToken(): string {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

export function writeToken(token: string): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, token)
  } catch {
    // 保存できなくてもこのタブでは使える。次に開いたときに再入力になる。
  }
}

export function clearToken(): void {
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // 消せなくても、この後の再読み込みで認証に失敗して入力を求められる。
  }
}
