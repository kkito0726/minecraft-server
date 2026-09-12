import { SPRITE_SIZE, SPRITES, spriteRuns } from './pixelSprites'
import type { PixelIconName } from './pixelSprites'

export type { PixelIconName } from './pixelSprites'

/**
 * 12×12 のドット絵のアイコン。
 *
 * 色は currentColor に従う。文字色を変えるだけで選択中・非選択の
 * 見た目を切り替えられる。常に飾りとして扱い、読み上げの対象にしない。
 * 意味は隣の文字が担う。
 */
export type PixelIconProps = {
  name: PixelIconName
  className?: string | undefined
}

export function PixelIcon({ name, className = 'size-5' }: PixelIconProps) {
  return (
    <svg
      viewBox={`0 0 ${SPRITE_SIZE} ${SPRITE_SIZE}`}
      aria-hidden="true"
      focusable="false"
      fill="currentColor"
      shapeRendering="crispEdges"
      className={className}
    >
      {spriteRuns(SPRITES[name]).map((r) => (
        <rect
          key={`${r.x}-${r.y}`}
          x={r.x}
          y={r.y}
          width={r.width}
          height={1}
          fillOpacity={r.opacity}
        />
      ))}
    </svg>
  )
}
