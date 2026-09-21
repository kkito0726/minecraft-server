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

/**
 * 画面で選ばせる「モード」。
 *
 * ハードコアは server.properties では gamemode と別のキーだが、遊ぶ側から
 * 見ればサバイバルの一種でしかない。別のチェック欄にすると
 * 「クリエイティブ + ハードコア」のような、意味の無い組み合わせを
 * 選べてしまう。ひとつの排他な選択にまとめる。
 */
export const HARDCORE_CHOICE = 'hardcore'

export const MODE_CHOICES: { value: string; label: string; hint: string }[] = [
  { value: String(GameMode.SURVIVAL), label: 'サバイバル', hint: '標準。体力と空腹があり、資源を集めて生き延びる' },
  {
    value: HARDCORE_CHOICE,
    label: 'ハードコア',
    hint: 'サバイバルの一種。死亡すると復帰できず、難易度はハードに固定される。後から外せない',
  },
  { value: String(GameMode.CREATIVE), label: 'クリエイティブ', hint: '資源が無限。飛行でき、ダメージを受けない' },
  { value: String(GameMode.ADVENTURE), label: 'アドベンチャー', hint: '専用の道具でしかブロックを壊せない。配布マップ向け' },
  { value: String(GameMode.SPECTATOR), label: 'スペクテイター', hint: '観戦のみ。ブロックに触れず、すり抜けて移動する' },
]

/** いまの値を選択肢の値にする。ハードコアが有効なら、モードより優先する。 */
export function modeChoiceValue(mode: GameMode, hardcore: boolean): string {
  return hardcore ? HARDCORE_CHOICE : String(mode)
}

/** 選択肢の値を、サーバーへ送る 2 つの値に戻す。 */
export function parseModeChoice(value: string): { mode: GameMode; hardcore: boolean } {
  if (value === HARDCORE_CHOICE) {
    // ハードコアはサバイバルに hardcore が付いたもの。
    return { mode: GameMode.SURVIVAL, hardcore: true }
  }
  return { mode: Number(value) as GameMode, hardcore: false }
}

/** ハードコアでは難易度がハードに固定される。画面でもそう見せる。 */
export const HARDCORE_DIFFICULTY = Difficulty.HARD
