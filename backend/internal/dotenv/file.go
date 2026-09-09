// Package dotenv は .env を「行の並び」として読み書きする。
//
// .env には日本語のコメントが 37 行以上あり、値の一部は source 互換のために
// 引用符で括られている（docs/backup-restore.md が「クォートを外すと source が
// 失敗する」と明記している）。map[string]string に読み込んで書き戻す実装だと
// これらが全部消えるため、値を変えない行は元のバイト列をそのまま書き戻す。
//
// File は不変。With / Without は新しい File を返し、元の File は変更しない。
package dotenv

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"
)

// File は .env の内容。不変。
type File struct {
	lines []Line
	// trailingNewline は元のファイルが改行で終わっていたか。
	// 書き戻したときに末尾の改行が増減しないようにするため保持する。
	trailingNewline bool
}

// Parse は .env を読む。
func Parse(r io.Reader) (*File, error) {
	scanner := bufio.NewScanner(r)
	// .env の 1 行は長くないが、壊れたファイルで固まらないよう上限を明示する
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		lines   []Line
		sawLine bool
	)
	for scanner.Scan() {
		sawLine = true
		lines = append(lines, parseLine(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(".env を読めません: %w", err)
	}

	return &File{lines: lines, trailingNewline: sawLine}, nil
}

// Get はキーの値を返す。第 2 戻り値はキーが存在したか。
func (f *File) Get(key string) (string, bool) {
	for _, l := range f.lines {
		if l.Kind == KindAssignment && l.Key == key {
			return l.Value, true
		}
	}
	return "", false
}

// Keys は登場順のキー一覧を返す。
func (f *File) Keys() []string {
	keys := make([]string, 0, len(f.lines))
	for _, l := range f.lines {
		if l.Kind == KindAssignment {
			keys = append(keys, l.Key)
		}
	}
	return keys
}

// With はキーの値を差し替えた新しい File を返す。元の File は変更しない。
//
// 既存のキーなら該当行の値だけを差し替える。キー名・行内コメント・
// 引用符の方針はそのまま維持されるので、周囲のコメントは一切動かない。
// 存在しないキーなら末尾に追加する。
func (f *File) With(key, value string) *File {
	lines := slices.Clone(f.lines)

	for i, l := range lines {
		if l.Kind == KindAssignment && l.Key == key {
			updated := l
			updated.Value = value
			updated.Raw = renderAssignment(updated)
			lines[i] = updated
			return &File{lines: lines, trailingNewline: f.trailingNewline}
		}
	}

	return &File{lines: appendNewKey(lines, key, value), trailingNewline: true}
}

// appendNewKey は末尾に「空行 + 代入行」を足す。
// 直前が既に空行なら重ねない。
func appendNewKey(lines []Line, key, value string) []Line {
	if len(lines) > 0 && lines[len(lines)-1].Kind != KindBlank {
		lines = append(lines, Line{Kind: KindBlank, Raw: ""})
	}
	added := Line{Kind: KindAssignment, Key: key, Value: value}
	added.Raw = renderAssignment(added)
	return append(lines, added)
}

// Without はキーの行を取り除いた新しい File を返す。
// キーに付随するコメント行は判別できないため残す（消すと情報が失われる）。
func (f *File) Without(key string) *File {
	lines := make([]Line, 0, len(f.lines))
	for _, l := range f.lines {
		if l.Kind == KindAssignment && l.Key == key {
			continue
		}
		lines = append(lines, l)
	}
	return &File{lines: lines, trailingNewline: f.trailingNewline}
}

// WriteTo は .env の内容を書き出す。
func (f *File) WriteTo(w io.Writer) (int64, error) {
	var sb strings.Builder
	for i, l := range f.lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(l.Raw)
	}
	if f.trailingNewline && len(f.lines) > 0 {
		sb.WriteByte('\n')
	}

	n, err := io.WriteString(w, sb.String())
	if err != nil {
		return int64(n), fmt.Errorf(".env を書き出せません: %w", err)
	}
	return int64(n), nil
}
