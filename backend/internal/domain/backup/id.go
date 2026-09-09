package backup

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidID はバックアップの識別子として使えない値であることを表す。
var ErrInvalidID = errors.New("バックアップの識別子が不正です")

// ID はバックアップの識別子。アーカイブのファイル名そのもの。
//
// ディレクトリ区切りを含まないことを保証する。これを許すと、
// 削除や復元の対象が保管ディレクトリの外を指せてしまう。
type ID struct {
	value string
}

// NewID はファイル名を検証して ID を作る。
func NewID(filename string) (ID, error) {
	switch {
	case filename == "":
		return ID{}, fmt.Errorf("%w: 空です", ErrInvalidID)
	case strings.ContainsAny(filename, `/\`):
		return ID{}, fmt.Errorf("%w: %q はディレクトリ区切りを含みます", ErrInvalidID, filename)
	case filename == "." || filename == "..":
		return ID{}, fmt.Errorf("%w: %q は使えません", ErrInvalidID, filename)
	case !strings.HasSuffix(filename, ".zip"):
		return ID{}, fmt.Errorf("%w: %q は .zip で終わっていません", ErrInvalidID, filename)
	default:
		return ID{value: filename}, nil
	}
}

// String はファイル名を返す。
func (id ID) String() string { return id.value }

// IsValid は有効な ID かを返す。ゼロ値は無効。
func (id ID) IsValid() bool { return id.value != "" }
