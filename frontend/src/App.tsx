import { AppShell } from './components/templates'

/**
 * アプリケーションのルート。
 *
 * 画面（サーバー状態 / ワールド / バックアップ）と操作の進捗購読は
 * フェーズ 13 以降で追加する。現在は骨組みの疎通確認のみ。
 */
export function App() {
  return (
    <AppShell brand="Minecraft サーバー管理コンソール" nav={null}>
      <p className="text-sm text-gray-600">画面は準備中です。</p>
    </AppShell>
  )
}
