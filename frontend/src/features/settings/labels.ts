import { Difficulty } from '../../gen/mcadmin/v1/server_pb'

/**
 * 難易度の表示。
 *
 * 違いを一言添える。名前だけだと「ノーマルとハードで何が変わるのか」が
 * 分からないまま選ぶことになる。
 */
export const DIFFICULTY_OPTIONS: { value: Difficulty; label: string; hint: string }[] = [
  { value: Difficulty.PEACEFUL, label: 'ピースフル', hint: '敵対するモブが出ない。空腹にならず体力も回復する' },
  { value: Difficulty.EASY, label: 'イージー', hint: '敵が弱い。空腹で体力が半分より減らない' },
  { value: Difficulty.NORMAL, label: 'ノーマル', hint: '標準。空腹で体力がハート半分まで減る' },
  { value: Difficulty.HARD, label: 'ハード', hint: '敵が強く、空腹で倒れることがある' },
]

export function difficultyLabel(difficulty: Difficulty): string {
  return DIFFICULTY_OPTIONS.find((o) => o.value === difficulty)?.label ?? '未設定'
}
