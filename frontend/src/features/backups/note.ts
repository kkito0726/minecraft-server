/**
 * バックアップのメモをファイル名に使える形にする。
 *
 * バックエンドの backup.NoteSlug の写し。規則の出典はあちらにあり、
 * ここは「押す前に結果を見せる」ためだけに持っている。
 * 日本語のメモは丸ごと消えるので、消えることを実行前に伝えたい。
 */

/** メモの上限。バックエンドの maxNoteLength と同じ。 */
export const MAX_NOTE_LENGTH = 32

const SAFE = /[A-Za-z0-9_]/

export function noteSlug(note: string): string {
  let out = ''
  let prevSeparator = true // 先頭のハイフンを抑止する

  for (const ch of note) {
    if (SAFE.test(ch)) {
      out += ch
      prevSeparator = false
      continue
    }
    if (!prevSeparator) {
      out += '-'
      prevSeparator = true
    }
  }

  const trimmed = trimHyphens(out)
  return trimmed.length > MAX_NOTE_LENGTH
    ? trimHyphens(trimmed.slice(0, MAX_NOTE_LENGTH))
    : trimmed
}

function trimHyphens(value: string): string {
  return value.replace(/^-+/, '').replace(/-+$/, '')
}
