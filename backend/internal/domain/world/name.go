// Package world はワールドのドメインモデル。
//
// この層は外部を一切知らない。ファイルシステムも docker も proto も import しない。
// ワールドが「何であるか」と「何をしてはいけないか」だけを表す。
package world

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ErrInvalidName はワールド名が規則に合わないことを表す。
var ErrInvalidName = errors.New("ワールド名が不正です")

// MaxNameLength はワールド名の最大長。
const MaxNameLength = 32

// NamePattern はワールド名の正規表現。
//
// フロントエンドの zod スキーマと同一の文字列を使う。片方だけ緩むと、
// 画面で弾かれない名前がファイルシステムに到達する（NFR-304）。
const NamePattern = `^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`

var nameRE = regexp.MustCompile(NamePattern)

// reservedNames は data/ 直下に実在するディレクトリ名。
// ワールド名として使うと本体のデータを壊す。
var reservedNames = []string{
	"cache",
	"config",
	"libraries",
	"logs",
	"plugins",
	"versions",
}

// quarantineMarkers は退避ディレクトリを表す接尾辞の目印。
// 退避されたディレクトリを通常のワールドとして扱わないため。
var quarantineMarkers = []string{".broken-", ".deleted-"}

// ReservedNames は予約名の一覧を返す。フロントエンドと共有するために公開している。
func ReservedNames() []string { return slices.Clone(reservedNames) }

// Name はワールド名。
//
// 値オブジェクトであり、生成できた時点で規則を満たしていることが保証される。
// これによって「検証を忘れた経路」が構造的に存在しなくなる。
type Name struct {
	value string
}

// NewName はワールド名を検証して作る。
func NewName(s string) (Name, error) {
	if err := validateName(s); err != nil {
		return Name{}, err
	}
	return Name{value: s}, nil
}

func validateName(s string) error {
	switch {
	case s == "":
		return fmt.Errorf("%w: 空です", ErrInvalidName)
	case len(s) > MaxNameLength:
		return fmt.Errorf("%w: %q は %d 文字を超えています", ErrInvalidName, s, MaxNameLength)
	case !nameRE.MatchString(s):
		return fmt.Errorf(
			"%w: %q。英数字で始まり、英数字・ハイフン・アンダースコアだけが使えます",
			ErrInvalidName, s)
	case isReserved(s):
		return fmt.Errorf("%w: %q は data/ が使う名前です", ErrInvalidName, s)
	case hasQuarantineMarker(s):
		return fmt.Errorf("%w: %q は退避ディレクトリの名前です", ErrInvalidName, s)
	default:
		return nil
	}
}

// isReserved は大文字小文字を区別せずに判定する。
// macOS のファイルシステムは既定で区別しないため、"LOGS" も logs と衝突する。
func isReserved(s string) bool {
	lower := strings.ToLower(s)
	return slices.Contains(reservedNames, lower)
}

func hasQuarantineMarker(s string) bool {
	for _, m := range quarantineMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// String はワールド名を返す。ゼロ値では空文字。
func (n Name) String() string { return n.value }

// IsValid は有効な Name かを返す。ゼロ値は無効。
func (n Name) IsValid() bool { return n.value != "" }
