package dotenv

import "strings"

// safeUnquotedRE は引用符なしで書いてよい値の文字集合。
// これ以外（空白・引用符・$・` など）を含む値はダブルクォートで括る。
// 括らないと source が単語分割したり変数展開したりして値が変わる。
const safeUnquotedChars = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"0123456789" +
	"_.:/=@%+,-"

// needsQuoting は値を引用符で括る必要があるかを返す。
// 空文字は括らない（KEY= がそのまま空文字になる）。
func needsQuoting(value string) bool {
	for i := 0; i < len(value); i++ {
		if !strings.ContainsRune(safeUnquotedChars, rune(value[i])) {
			return true
		}
	}
	return false
}

// formatValue は値を .env に書ける形にする。
//
// 元の行が引用符で括られていたなら、値が安全でもその引用符を維持する
// （MC_MOTD のように「引用してある」こと自体が意図である場合を壊さない）。
func formatValue(value string, original Quote) (string, Quote) {
	switch {
	case original == QuoteSingle && !strings.Contains(value, "'"):
		return "'" + value + "'", QuoteSingle
	case original != QuoteNone || needsQuoting(value):
		return `"` + escapeDouble(value) + `"`, QuoteDouble
	default:
		return value, QuoteNone
	}
}

// escapeDouble はダブルクォート内で特別な意味を持つ文字を退避する。
// $ と ` を escape しないと source 時に変数展開・コマンド置換が起きる。
func escapeDouble(value string) string {
	var sb strings.Builder
	sb.Grow(len(value) + 8)
	for i := 0; i < len(value); i++ {
		switch c := value[i]; c {
		case '\\', '"', '$', '`':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		case '\n':
			sb.WriteString(`\n`)
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// renderAssignment は代入行を組み立てる。
// 行内コメントは元の空白ごと保持する。
func renderAssignment(l Line) string {
	formatted, _ := formatValue(l.Value, l.Quote)
	return l.Key + "=" + formatted + l.TrailingComment
}
