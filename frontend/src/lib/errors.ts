import { Code, ConnectError } from '@connectrpc/connect'

/**
 * 通信のコードごとの案内。
 *
 * サーバーがメッセージを載せてこなかった場合だけ使う。バックエンドは
 * 利用者が対処できるエラー（名前の打ち間違い、容量不足、承諾漏れ）に
 * 限って日本語の文言を返すので、それがあるならそちらを優先する。
 */
const FALLBACK: Partial<Record<Code, string>> = {
  [Code.Unauthenticated]: 'トークンで認証できませんでした。',
  [Code.PermissionDenied]: 'トークンで認証できませんでした。',
  [Code.Unavailable]: 'サーバーに接続できません。mcadmind が動いているか確認してください。',
  [Code.DeadlineExceeded]: '時間内に完了しませんでした。',
  [Code.FailedPrecondition]: '今はこの操作を実行できません。',
  [Code.Canceled]: '操作が中断されました。',
}

const UNKNOWN = '不明なエラーが発生しました。'

/** エラーを画面に出す 1 行の日本語にする。 */
export function describeError(err: unknown): string {
  if (err instanceof ConnectError) {
    return err.rawMessage.trim() || FALLBACK[err.code] || UNKNOWN
  }
  if (err instanceof Error) {
    return err.message.trim() || UNKNOWN
  }
  if (typeof err === 'string') {
    return err.trim() || UNKNOWN
  }
  return UNKNOWN
}

/**
 * トークンが受け付けられなかったかを返す。
 *
 * 認可の失敗はトークンの入力し直しで直る唯一のエラーなので、
 * 他のエラーとは別に扱う必要がある。
 */
export function isUnauthenticated(err: unknown): boolean {
  if (!(err instanceof ConnectError)) {
    return false
  }
  return err.code === Code.Unauthenticated || err.code === Code.PermissionDenied
}
