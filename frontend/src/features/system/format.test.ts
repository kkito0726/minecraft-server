import { describe, expect, it } from 'vitest'

import { loadLabel, percentLabel, usageTone, windowLabel } from './format'

describe('usageTone', () => {
  // 4GB の Pi は MC だけで実 RSS 約 2.7GB を使う。80% を警告にすると
  // 常時警告になって意味を失うので、90% から警告にする。
  it('70% 台は通常として扱う', () => {
    expect(usageTone(72)).toBe('ok')
    expect(usageTone(89.9)).toBe('ok')
  })

  it('90% から警告、95% から危険', () => {
    expect(usageTone(90)).toBe('warn')
    expect(usageTone(94.9)).toBe('warn')
    expect(usageTone(95)).toBe('danger')
    expect(usageTone(100)).toBe('danger')
  })
})

describe('percentLabel', () => {
  it('小数 1 桁で出す', () => {
    expect(percentLabel(42.51)).toBe('42.5%')
    expect(percentLabel(99.96)).toBe('100.0%')
    expect(percentLabel(0)).toBe('0.0%')
  })

  it('数でなければ伏せる', () => {
    expect(percentLabel(Number.NaN)).toBe('—')
  })
})

describe('windowLabel', () => {
  // 1 点の値ではなく差分であることを隠さない。瞬間値だと思って
  // 判断されると、短い山も谷も見えていないことに気づけない。
  it('秒と分で言い換える', () => {
    expect(windowLabel(5)).toBe('直近 5 秒の平均')
    expect(windowLabel(59)).toBe('直近 59 秒の平均')
    expect(windowLabel(120)).toBe('直近 2 分の平均')
  })

  it('区間が無ければ空にする', () => {
    expect(windowLabel(0)).toBe('')
    expect(windowLabel(Number.NaN)).toBe('')
  })
})

describe('loadLabel', () => {
  // コア数が無いと、1.5 が高いのか低いのか判断できない。
  it('コア数を添える', () => {
    expect(loadLabel(1.2, 0.9, 0.7, 4)).toBe('1.20 / 0.90 / 0.70（4 コア）')
  })

  it('コア数が分からなければ値だけ出す', () => {
    expect(loadLabel(1.2, 0.9, 0.7, 0)).toBe('1.20 / 0.90 / 0.70')
  })
})
