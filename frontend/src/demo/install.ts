/**
 * デモを差し込む。
 *
 * 通信の口を丸ごと入れ替えるだけで、画面のコードには一切触らない。
 * このモジュールは `__DEMO__` が真のビルドからしか読み込まれない。
 */
import { setTransport } from '../lib/transport'
import { createDemoTransport } from './transport'

export function installDemo(): void {
  setTransport(createDemoTransport())
}
