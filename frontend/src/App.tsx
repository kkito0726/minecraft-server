/**
 * アプリケーションのルート。
 *
 * 画面（サーバー状態 / ワールド / バックアップ）と操作の進捗購読は
 * フェーズ 13 以降で追加する。現在はツールチェーンの疎通確認のみ。
 */
export function App() {
  return (
    <main className="flex h-full items-center justify-center p-8">
      <div className="max-w-md text-center">
        <h1 className="text-xl font-semibold">Minecraft サーバー管理コンソール</h1>
        <p className="mt-2 text-sm text-gray-600">
          画面は準備中です。
        </p>
      </div>
    </main>
  )
}
