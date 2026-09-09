/**
 * 確認入力が期待値と一致しているかを返す。実行ボタンの活性判定に使う。
 *
 * コンポーネントと別ファイルにしているのは、React Fast Refresh が
 * 「コンポーネントだけを export するファイル」でしか働かないため。
 */
export function isConfirmed(expected: string, value: string): boolean {
  return expected.length > 0 && expected === value
}
