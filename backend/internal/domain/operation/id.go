package operation

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidID は操作の識別子として使えない値であることを表す。
var ErrInvalidID = errors.New("操作の識別子が不正です")

// ID は操作の識別子。
type ID struct {
	value string
}

// NewID は識別子を検証して作る。
func NewID(s string) (ID, error) {
	if strings.TrimSpace(s) == "" {
		return ID{}, fmt.Errorf("%w: 空です", ErrInvalidID)
	}
	return ID{value: s}, nil
}

// MustID はテストと内部生成のための版。不正な値では panic する。
func MustID(s string) ID {
	id, err := NewID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// String は識別子を返す。
func (id ID) String() string { return id.value }

// IsValid は有効な ID かを返す。ゼロ値は無効。
func (id ID) IsValid() bool { return id.value != "" }
