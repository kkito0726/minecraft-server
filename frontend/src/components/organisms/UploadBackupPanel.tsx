import { useRef, useState } from 'react'

import { Button, ProgressBar } from '../atoms'

/**
 * 外から持ち込んだ zip の取り込み。
 *
 * 取り込むだけで、ワールドには触らない。差し替えるかどうかは、この後の
 * 復元でこれまでどおりの関門（バージョンの確認と名前の入力）を通して
 * 決める。「置くこと」と「使うこと」を同時に決めさせない。
 */
export type UploadBackupPanelProps = {
  disabled?: boolean | undefined
  /** 0〜1。送信中でなければ null。 */
  progress: number | null
  error?: string | undefined
  /** 取り込みが終わったときの案内。 */
  notice?: string | undefined
  onUpload: (file: File) => void
}

export function UploadBackupPanel({
  disabled,
  progress,
  error,
  notice,
  onUpload,
}: UploadBackupPanelProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [selected, setSelected] = useState<File | null>(null)
  const busy = progress !== null

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (selected) {
      onUpload(selected)
    }
  }

  return (
    <form
      className="hud-inset flex flex-col gap-4"
      aria-label="バックアップの取り込み"
      onSubmit={submit}
    >
      <p aria-hidden="true" className="hud-tag">
        IMPORT // EXTERNAL ZIP
      </p>
      <p className="text-xs text-dim">
        手元の zip を保管先に取り込みます。ワールドはまだ差し替わりません。
        取り込んだあと、一覧から復元してください。
      </p>

      <FileField
        inputRef={inputRef}
        selected={selected}
        disabled={disabled || busy}
        onSelect={setSelected}
      />

      {busy && <ProgressBar done={Math.round(progress * 100)} total={100} label="送信の進み具合" />}

      {error && (
        <p role="alert" className="text-sm text-danger-ink">
          {error}
        </p>
      )}
      {notice && !error && <p className="text-sm text-ok-ink">{notice}</p>}

      <div>
        <Button type="submit" tone="primary" disabled={disabled || busy || selected === null}>
          {busy ? '送信しています…' : '取り込む'}
        </Button>
      </div>
    </form>
  )
}

type FileFieldProps = {
  inputRef: React.RefObject<HTMLInputElement | null>
  selected: File | null
  disabled: boolean | undefined
  onSelect: (file: File | null) => void
}

function FileField({ inputRef, selected, disabled, onSelect }: FileFieldProps) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor="import-file" className="text-xs font-semibold tracking-wider text-dim">
        取り込む zip
      </label>
      <input
        id="import-file"
        ref={inputRef}
        type="file"
        accept=".zip,application/zip"
        disabled={disabled}
        className={[
          'text-sm text-dim disabled:cursor-not-allowed disabled:opacity-50',
          'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-diamond',
          'file:mr-3 file:cursor-pointer file:border file:border-line-strong file:bg-raised',
          'file:px-3 file:py-1.5 file:text-xs file:font-semibold file:text-fg',
          'hover:file:border-emerald',
        ].join(' ')}
        onChange={(e) => onSelect(e.target.files?.[0] ?? null)}
        aria-describedby="import-file-hint"
      />
      <p id="import-file-hint" className="text-xs text-faint">
        {selected
          ? `${selected.name}（${Math.max(1, Math.round(selected.size / 1024))} KB）`
          : 'data/<名前>/ か <名前>/ の形で level.dat を含むものを選んでください。'}
      </p>
    </div>
  )
}
