package dotenv

import (
	"regexp"
	"strings"
)

// LineKind は 1 行の種類。
type LineKind int

const (
	// KindBlank は空行（空白のみを含む行も）。
	KindBlank LineKind = iota
	// KindComment は # で始まる行。
	KindComment
	// KindAssignment は KEY=VALUE の行。
	KindAssignment
	// KindUnknown はどれにも当てはまらない行。内容をそのまま保つ。
	KindUnknown
)

// Quote は値を囲っている引用符の種類。囲っていなければ QuoteNone。
type Quote byte

const (
	// QuoteNone は引用符なし。
	QuoteNone Quote = 0
	// QuoteDouble はダブルクォート。
	QuoteDouble Quote = '"'
	// QuoteSingle はシングルクォート。
	QuoteSingle Quote = '\''
)

// Line は .env の 1 行。
//
// Raw を必ず保持するのが要点。値を変えない行は Raw をバイト単位でそのまま
// 書き戻すため、日本語コメントも空行も引用符も一切触れられない。
type Line struct {
	Kind LineKind
	// Raw は元の行（改行を含まない）。
	Raw string
	// Key は KindAssignment のときのキー名。
	Key string
	// Value は引用符とエスケープを解いた値。
	Value string
	// Quote は元の行で値を囲っていた引用符。
	Quote Quote
	// TrailingComment は値の後ろに続くコメント（"  # 説明" のような形。前後の空白を含む）。
	TrailingComment string
}

// assignmentRE は KEY=VALUE の行を捉える。
// export 接頭辞は .env では使っていないので受け付けない。
var assignmentRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// parseLine は 1 行を解釈する。Raw は常に元の内容のまま保持する。
func parseLine(raw string) Line {
	trimmed := strings.TrimSpace(raw)

	switch {
	case trimmed == "":
		return Line{Kind: KindBlank, Raw: raw}
	case strings.HasPrefix(trimmed, "#"):
		return Line{Kind: KindComment, Raw: raw}
	}

	m := assignmentRE.FindStringSubmatch(raw)
	if m == nil {
		return Line{Kind: KindUnknown, Raw: raw}
	}

	value, quote, comment := splitValue(m[2])
	return Line{
		Kind:            KindAssignment,
		Raw:             raw,
		Key:             m[1],
		Value:           value,
		Quote:           quote,
		TrailingComment: comment,
	}
}

// splitValue は = の右側を、値・引用符・行内コメントに分ける。
//
// 引用符なしの場合、シェルと同じく「空白に続く #」以降をコメントとみなす。
// EQUALS_IN_VALUE=a=b=c のように = を含む値は、最初の = だけで分割済みなので
// ここには a=b=c が渡り、そのまま値になる。
func splitValue(rest string) (value string, quote Quote, comment string) {
	rest = strings.TrimLeft(rest, " \t")

	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		q := Quote(rest[0])
		if v, after, ok := scanQuoted(rest, byte(q)); ok {
			return v, q, after
		}
		// 閉じていない引用符。値として素直に扱う。
		return rest, QuoteNone, ""
	}

	if i := indexUnquotedComment(rest); i >= 0 {
		return strings.TrimRight(rest[:i], " \t"), QuoteNone, rest[i:]
	}
	return strings.TrimRight(rest, " \t"), QuoteNone, ""
}

// scanQuoted は引用符で囲まれた値を読み取り、値と残りを返す。
func scanQuoted(s string, q byte) (value, rest string, ok bool) {
	var sb strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		// シングルクォートの中ではエスケープは効かない（シェルと同じ）
		if c == '\\' && q == '"' && i+1 < len(s) {
			i++
			sb.WriteByte(unescapeDouble(s[i]))
			continue
		}
		if c == q {
			return sb.String(), s[i+1:], true
		}
		sb.WriteByte(c)
	}
	return "", "", false
}

func unescapeDouble(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	default:
		// \" \\ \$ \` はその文字自身になる
		return c
	}
}

// indexUnquotedComment は「空白に続く #」について、その空白列の先頭位置を返す。
// 無ければ -1。行頭が # の場合はコメント行として先に分岐しているのでここには来ない。
//
// 空白列の先頭まで遡るのが要点。# の直前 1 文字だけを見ると、
// "K=v  # 説明" のような 2 個以上の空白が 1 個に縮んでしまう。
func indexUnquotedComment(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] != '#' || !isSpace(s[i-1]) {
			continue
		}
		start := i - 1
		for start > 0 && isSpace(s[start-1]) {
			start--
		}
		return start
	}
	return -1
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' }
