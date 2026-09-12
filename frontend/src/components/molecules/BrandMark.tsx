import { PixelIcon } from '../atoms'

/**
 * アプリの名札。光るブロックと、正式な名前。
 *
 * 英字の「MCADMIN」は飾りで、読み上げには正式な名前だけを渡す。
 */
export type BrandMarkProps = {
  title: string
}

export function BrandMark({ title }: BrandMarkProps) {
  return (
    <div className="flex items-center gap-3">
      <PixelIcon
        name="block"
        className="size-9 shrink-0 text-emerald drop-shadow-[0_0_10px_var(--color-emerald)]"
      />
      <div className="flex min-w-0 flex-col gap-1.5">
        <span aria-hidden="true" className="font-pixel text-lg leading-none tracking-[0.18em] text-fg">
          MCADMIN
        </span>
        {/* 日本語の途中で折り返さない。折るなら英語との境目の空白で。 */}
        <span className="text-[11px] leading-tight break-keep text-faint">{title}</span>
      </div>
    </div>
  )
}
