import { Button } from '../atoms'

/**
 * 保持ポリシーの手動適用。
 *
 * まず dry_run で対象を見せてから消す。「押したら何が消えるか
 * 分からない」状態で削除させない。
 */
export type PrunePanelProps = {
  /** dry_run の結果。まだ確認していなければ null。 */
  preview: string[] | null
  disabled?: boolean | undefined
  onPreview: () => void
  onApply: () => void
}

export function PrunePanel({ preview, disabled, onPreview, onApply }: PrunePanelProps) {
  return (
    <div className="flex flex-col gap-2 border-t border-gray-100 pt-3">
      <p className="text-xs text-gray-500">
        保持ポリシーは取得のたびに自動で適用されます。今すぐ適用したいときだけ使ってください。
      </p>

      <div className="flex gap-2">
        <Button disabled={disabled} onClick={onPreview}>
          対象を確認
        </Button>
        {preview !== null && preview.length > 0 && (
          <Button tone="danger" disabled={disabled} onClick={onApply}>
            削除する
          </Button>
        )}
      </div>

      {preview !== null &&
        (preview.length === 0 ? (
          <p className="text-sm text-gray-600">削除するものはありません。</p>
        ) : (
          <ul className="flex flex-col gap-1 text-sm text-gray-700">
            {preview.map((id) => (
              <li key={id}>
                <code>{id}</code>
              </li>
            ))}
          </ul>
        ))}
    </div>
  )
}
