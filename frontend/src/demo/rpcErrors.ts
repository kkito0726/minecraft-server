/**
 * デモが返すエラー。
 *
 * 実物と同じ Connect のコードで返す。画面は describeError で文言を
 * 組み立てるので、コードが違うと出てくる案内も変わってしまう。
 */
import { Code, ConnectError } from '@connectrpc/connect'

import { DemoBusyError } from './operations'

/** 他の操作が実行中。実物のロック衝突にあたる。 */
export function busyGuard<T>(fn: () => T): T {
  try {
    return fn()
  } catch (err) {
    if (err instanceof DemoBusyError) {
      throw new ConnectError(err.message, Code.FailedPrecondition)
    }
    throw err
  }
}

export function notFound(message: string): ConnectError {
  return new ConnectError(message, Code.NotFound)
}

export function invalidArgument(message: string): ConnectError {
  return new ConnectError(message, Code.InvalidArgument)
}

export function failedPrecondition(message: string): ConnectError {
  return new ConnectError(message, Code.FailedPrecondition)
}
