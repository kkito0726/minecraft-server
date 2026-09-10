/**
 * 各ルートの仮置き。
 *
 * 画面の中身はフェーズ 15〜17 で作る。ここではルーティングと
 * 認証の疎通だけを確かめられるようにしておく。
 */
function Placeholder({ title }: { title: string }) {
  return (
    <section>
      <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
      <p className="mt-2 text-sm text-gray-600">この画面は準備中です。</p>
    </section>
  )
}

export function BackupsPage() {
  return <Placeholder title="バックアップ" />
}
