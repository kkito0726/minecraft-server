// Package shared は複数の集約にまたがる値オブジェクトを置く。
//
// ここも外部を知らない。標準ライブラリ以外を import しない。
package shared

import (
	"errors"
	"fmt"
)

// ErrInvalidDataVersion は DataVersion として使えない値であることを表す。
var ErrInvalidDataVersion = errors.New("DataVersion が不正です")

// DataVersion は level.dat の Data.DataVersion。
//
// バージョン比較のキーはこの整数であって、表示文字列 Data.Version.Name ではない。
// 表示文字列は同じ "26.2" でもスナップショット間で中身が異なることがあり、
// ワールドのアップグレードは片道なので取り違えると元に戻せない。
//
// int32 の別名ではなく構造体にしているのは、うっかり生の整数や
// 文字列と比較するコードがコンパイルを通らないようにするため。
type DataVersion struct {
	value int32
}

// NewDataVersion は DataVersion を作る。
func NewDataVersion(v int32) (DataVersion, error) {
	if v <= 0 {
		return DataVersion{}, fmt.Errorf("%w: %d（正の整数である必要があります）", ErrInvalidDataVersion, v)
	}
	return DataVersion{value: v}, nil
}

// Int32 は生の値を返す。永続化と転送のためだけに使う。
func (v DataVersion) Int32() int32 { return v.value }

// LessThan は自分が引数より古いかを返す。
func (v DataVersion) LessThan(other DataVersion) bool { return v.value < other.value }

// Equals は同じバージョンかを返す。
func (v DataVersion) Equals(other DataVersion) bool { return v.value == other.value }

// WorldVersion は level.dat から読んだワールドのバージョン情報。
//
// 「読めなかった」状態を表現できることが要点。読めなかったことを
// バージョン 0 で表すと、破損したワールドを最古のものとして比較してしまう。
type WorldVersion struct {
	readable    bool
	name        string
	dataVersion DataVersion
	snapshot    bool
	levelName   string
}

// NewWorldVersion は読み取れたバージョンを表す WorldVersion を作る。
func NewWorldVersion(name string, dv DataVersion, snapshot bool, levelName string) WorldVersion {
	return WorldVersion{
		readable:    true,
		name:        name,
		dataVersion: dv,
		snapshot:    snapshot,
		levelName:   levelName,
	}
}

// UnreadableWorldVersion は level.dat を読めなかったことを表す WorldVersion を返す。
func UnreadableWorldVersion() WorldVersion { return WorldVersion{} }

// Readable は level.dat を読めたかを返す。
func (v WorldVersion) Readable() bool { return v.readable }

// Name は表示用のバージョン文字列を返す。比較には使わない。
func (v WorldVersion) Name() string { return v.name }

// DataVersion は比較に使うバージョンを返す。読めていなければ第 2 戻り値が偽。
func (v WorldVersion) DataVersion() (DataVersion, bool) {
	if !v.readable {
		return DataVersion{}, false
	}
	return v.dataVersion, true
}

// Snapshot はスナップショット版かを返す。
func (v WorldVersion) Snapshot() bool { return v.snapshot }

// LevelName は level.dat に記録されたワールド名を返す。
// ディレクトリ名と一致しないことがある。
func (v WorldVersion) LevelName() string { return v.levelName }

// Display は画面に出す文字列を返す。
// 読めなかった場合に空文字を返すと「バージョンが空のワールド」に見えるため、
// 明示的に「不明」と表示する。
func (v WorldVersion) Display() string {
	if !v.readable {
		return "不明"
	}
	if v.snapshot {
		return v.name + "（スナップショット）"
	}
	return v.name
}
