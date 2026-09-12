import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { PixelIcon } from './PixelIcon'
import { SPRITE_SIZE, SPRITES, spriteRuns } from './pixelSprites'
import type { PixelIconName } from './pixelSprites'

const NAMES = Object.keys(SPRITES) as PixelIconName[]

describe('spriteRuns', () => {
  it('同じ濃さが横に続く画素を 1 本にまとめる', () => {
    expect(spriteRuns(['aab.'])).toEqual([
      { x: 0, y: 0, width: 2, opacity: 1 },
      { x: 2, y: 0, width: 1, opacity: 0.55 },
    ])
  })

  it('透明の画素は矩形にしない', () => {
    expect(spriteRuns(['....', '.c..'])).toEqual([{ x: 1, y: 1, width: 1, opacity: 0.28 }])
  })
})

describe('SPRITES', () => {
  // 1 文字でも欠けると、その行だけ左に寄って絵が崩れる。
  it.each(NAMES)('%s は 12×12 の枠ちょうどに描かれている', (name) => {
    const rows = SPRITES[name]

    expect(rows).toHaveLength(SPRITE_SIZE)
    for (const row of rows) {
      expect(row).toHaveLength(SPRITE_SIZE)
    }
  })

  it.each(NAMES)('%s は決められた文字だけでできている', (name) => {
    expect(SPRITES[name].join('')).toMatch(/^[abc.]+$/)
  })
})

describe('PixelIcon', () => {
  // 意味は隣の文字が担う。アイコンまで読み上げると同じことを二度言う。
  it.each(NAMES)('%s は読み上げから外れた絵になる', (name) => {
    const { container } = render(<PixelIcon name={name} />)
    const svg = container.querySelector('svg')

    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg).toHaveAttribute('viewBox', '0 0 12 12')
    expect(svg?.querySelectorAll('rect').length).toBeGreaterThan(0)
  })

  it('大きさは呼び出し側が決められる', () => {
    const { container } = render(<PixelIcon name="block" className="size-20" />)

    expect(container.querySelector('svg')).toHaveClass('size-20')
  })
})
