import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { App } from './App'
import './styles/index.css'

const container = document.getElementById('root')
if (!container) {
  throw new Error('#root が見つかりません。index.html を確認してください。')
}

/**
 * 画面を出す前に、デモなら通信の口を差し替える。
 *
 * 動的 import にしてあるのは、通常のビルドからデモの実装を丸ごと
 * 落とすため。__DEMO__ は定数に畳まれるので、条件ごと消える。
 */
async function boot(root: HTMLElement): Promise<void> {
  if (__DEMO__) {
    const { installDemo } = await import('./demo/install')
    installDemo()
  }

  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}

void boot(container)
