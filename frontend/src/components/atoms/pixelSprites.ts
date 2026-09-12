/**
 * PixelIcon の絵。12×12 の文字列で持つ。
 *
 * 濃淡は 3 段（a: 塗り / b: 中間 / c: 暗部）、'.' は透明。
 * 色は描く側の currentColor に従うので、ここは形と濃淡だけを決める。
 */
export type PixelIconName = 'block' | 'server' | 'world' | 'backup'

export const SPRITE_SIZE = 12

export const SPRITES: Record<PixelIconName, readonly string[]> = {
  // 等角のブロック。上面・左面・右面の 3 面で立体に見せる。
  block: [
    '.....aa.....',
    '...aaaaaa...',
    '.aaaaaaaaaa.',
    'baaaaaaaaaac',
    'bbbaaaaaaccc',
    'bbbbbaaccccc',
    'bbbbbbcccccc',
    'bbbbbbcccccc',
    'bbbbbbcccccc',
    '.bbbbbccccc.',
    '...bbbccc...',
    '.....bc.....',
  ],
  // 2 段のサーバー筐体。差し込み口と点灯中の灯り。
  server: [
    'aaaaaaaaaaaa',
    'acccccccccca',
    'acaacccccaca',
    'acccccccccca',
    'aaaaaaaaaaaa',
    '............',
    'aaaaaaaaaaaa',
    'acccccccccca',
    'acaacccccaca',
    'acccccccccca',
    'aaaaaaaaaaaa',
    '............',
  ],
  // 草のブロック。上に草、垂れた草の下は土。
  world: [
    'aaaaaaaaaaaa',
    'aaaaaaaaaaaa',
    'aaaaaaaaaaaa',
    'aacaaaacaaac',
    'cacacccaccac',
    'cccccbcccccc',
    'cbcccccccbcc',
    'ccccccbccccc',
    'cccbcccccccb',
    'cccccccbcccc',
    'cbcccccccccc',
    'ccccbcccccbc',
  ],
  // チェスト。蓋と胴の継ぎ目に留め金。
  backup: [
    '............',
    '.aaaaaaaaaa.',
    '.abbbbbbbba.',
    '.abbbbbbbba.',
    '.abbbaabbba.',
    '.aaaaaaaaaa.',
    '.acccaaccca.',
    '.acccccccca.',
    '.acccccccca.',
    '.acccccccca.',
    '.aaaaaaaaaa.',
    '............',
  ],
}

const SHADE: Record<string, number> = { a: 1, b: 0.55, c: 0.28 }

export type SpriteRun = { x: number; y: number; width: number; opacity: number }

/** 同じ濃さが横に続く画素を 1 本の矩形にまとめる。要素数を減らすため。 */
export function spriteRuns(rows: readonly string[]): SpriteRun[] {
  return rows.flatMap((row, y) => {
    const runs: SpriteRun[] = []
    let x = 0
    while (x < row.length) {
      const ch = row[x]!
      let end = x + 1
      while (end < row.length && row[end] === ch) {
        end++
      }
      const opacity = SHADE[ch]
      if (opacity !== undefined) {
        runs.push({ x, y, width: end - x, opacity })
      }
      x = end
    }
    return runs
  })
}
