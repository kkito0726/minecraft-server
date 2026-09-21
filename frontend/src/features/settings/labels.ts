import { Difficulty, GameMode } from '../../gen/mcadmin/v1/server_pb'

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

/**
 * ゲームモードの表示。
 *
 * **サーバー全体の設定で、ワールドごとの属性ではない。** 切り替えても
 * 付いてこないことは、選ばせる画面の側で必ず添える。
 */
export const GAME_MODE_OPTIONS: { value: GameMode; label: string; hint: string }[] = [
  { value: GameMode.SURVIVAL, label: 'サバイバル', hint: '標準。体力と空腹があり、資源を集めて生き延びる' },
  { value: GameMode.CREATIVE, label: 'クリエイティブ', hint: '資源が無限。飛行でき、ダメージを受けない' },
  { value: GameMode.ADVENTURE, label: 'アドベンチャー', hint: '専用の道具でしかブロックを壊せない。配布マップ向け' },
  { value: GameMode.SPECTATOR, label: 'スペクテイター', hint: '観戦のみ。ブロックに触れず、すり抜けて移動する' },
]

export function gameModeLabel(mode: GameMode): string {
  return GAME_MODE_OPTIONS.find((o) => o.value === mode)?.label ?? '未設定'
}
