import { describe, expect, it } from 'vitest'

import { isConfirmed } from './confirmation'

/**
 * 復元とワールド削除は取り消せない。ここが緩むと、押し間違いが
 * そのままデータの喪失になる。完全一致だけを通す。
 */
describe('isConfirmed', () => {
  it.each([
    ['完全に一致', 'world', 'world', true],
    ['大文字小文字違い', 'world', 'World', false],
    ['前後に空白', 'world', ' world ', false],
    ['途中まで', 'world', 'wor', false],
    ['余分な文字', 'world', 'world2', false],
    ['入力が空', 'world', '', false],
    ['期待値が空なら常に不成立', '', '', false],
  ])('%s', (_name, expected, value, want) => {
    expect(isConfirmed(expected, value)).toBe(want)
  })
})
