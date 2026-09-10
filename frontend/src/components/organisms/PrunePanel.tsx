import { Button } from '../atoms'

/**
 * 保持ポリシーの手動適用。
 *
 * まず dry_run で対象を見せてから消す。「押したら何が消えるか
 * 分からない」状態で削除させない。
 */
export type PruneResult =
  /** dry_run の結果。これから消える予定のもの。 */
  | { kind: 'preview'; ids: string[] }
  /** 実際に消したもの。予定と食い違うことがあるので、こちらを正として出す。 */
  | { kind: 'applied'; ids: string[] }

export type PrunePanelProps = {
  /** まだ何もしていなければ null。 */
  result: PruneResult | null
  disabled?: boolean | undefined
  onPreview: () => void
  onApply: () => void
}

export function PrunePanel({ result, disabled, onPreview, onApply }: PrunePanelProps) {
  return (
    <div className="flex flex-col gap-2 border-t border-gray-100 pt-3">
      <p className="text-xs text-gray-500">
        保持ポリシーは取得のたびに自動で適用されます。今すぐ適用したいときだけ使ってください。
      </p>

      <div className="flex gap-2">
        <Button disabled={disabled} onClick={onPreview}>
          対象を確認
        </Button>
        {/*
          削除を出すのは「これから消えるもの」を見せているときだけ。
          消したあとの一覧に対して押せると、確認していない集合を
          消すことになる（確認のあとに取得が走れば対象は変わる）。
        */}
        {result?.kind === 'preview' && result.ids.length > 0 && (
          <Button tone="danger" disabled={disabled} onClick={onApply}>
            削除する
          </Button>
        )}
      </div>

      {result && <PruneResultView result={result} />}
    </div>
  )
}

function PruneResultView({ result }: { result: PruneResult }) {
  if (result.ids.length === 0) {
    return (
      <p className="text-sm text-gray-600">
        {result.kind === 'preview' ? '削除するものはありません。' : '削除したものはありません。'}
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      <p className="text-sm text-gray-700">
        {result.kind === 'preview'
          ? `次の ${result.ids.length} 件が削除の対象です。`
          : `次の ${result.ids.length} 件を削除しました。`}
      </p>
      <ul className="flex flex-col gap-1 text-sm text-gray-700">
        {result.ids.map((id) => (
          <li key={id}>
            <code>{id}</code>
          </li>
        ))}
      </ul>
    </div>
  )
}
