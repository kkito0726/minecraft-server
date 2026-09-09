package leveldat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// NBT のタグ種別。
//
// level.dat から取り出したいのは文字列・整数・長整数・バイトの 4 種だけだが、
// 目的のフィールドに辿り着くには他の型も正しく読み飛ばす必要があるため、
// 全種別のサイズ計算を実装している。
const (
	tagEnd byte = iota
	tagByte
	tagShort
	tagInt
	tagLong
	tagFloat
	tagDouble
	tagByteArray
	tagString
	tagList
	tagCompound
	tagIntArray
	tagLongArray
)

// maxPayloadLen は配列や文字列の長さの上限。
//
// NBT の長さは int32 なので、壊れたデータが 2GB の確保を要求しうる。
// level.dat は実測 637 バイトで、Pi では MC が既に 2.7GB 使っているため、
// 現実的な上限で弾く。
const maxPayloadLen = 16 << 20 // 16MiB

// errTruncated は入力が途中で終わっていることを表す。
var errTruncated = errors.New("NBT データが途中で終わっています")

// reader は NBT を前から順に読む。
//
// 木を丸ごとメモリに展開せず、必要なフィールドだけ拾って残りは読み飛ばす。
type reader struct {
	buf []byte
	pos int
}

func (r *reader) remaining() int { return len(r.buf) - r.pos }

func (r *reader) take(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, errTruncated
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *reader) byteValue() (byte, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *reader) uint16Value() (uint16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *reader) int32Value() (int32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

func (r *reader) int64Value() (int64, error) {
	b, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}

// stringValue は uint16 の長さに続く UTF-8 文字列を読む。
func (r *reader) stringValue() (string, error) {
	n, err := r.uint16Value()
	if err != nil {
		return "", err
	}
	b, err := r.take(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// arrayLen は配列の要素数を読む。負の値と過大な値を弾く。
func (r *reader) arrayLen() (int, error) {
	n, err := r.int32Value()
	if err != nil {
		return 0, err
	}
	if n < 0 || int(n) > maxPayloadLen {
		return 0, fmt.Errorf("NBT の配列長が不正です: %d", n)
	}
	return int(n), nil
}

// skipPayload は指定した型の値を読み飛ばす。
func (r *reader) skipPayload(tag byte) error {
	switch tag {
	case tagByte:
		return r.skipBytes(1)
	case tagShort:
		return r.skipBytes(2)
	case tagInt, tagFloat:
		return r.skipBytes(4)
	case tagLong, tagDouble:
		return r.skipBytes(8)
	case tagString:
		n, err := r.uint16Value()
		if err != nil {
			return err
		}
		return r.skipBytes(int(n))
	case tagByteArray:
		return r.skipArray(1)
	case tagIntArray:
		return r.skipArray(4)
	case tagLongArray:
		return r.skipArray(8)
	case tagList:
		return r.skipList()
	case tagCompound:
		return r.skipCompound()
	case tagEnd:
		return nil
	default:
		return fmt.Errorf("NBT に未知のタグ種別があります: %d", tag)
	}
}

func (r *reader) skipBytes(n int) error {
	_, err := r.take(n)
	return err
}

func (r *reader) skipArray(elemSize int) error {
	n, err := r.arrayLen()
	if err != nil {
		return err
	}
	return r.skipBytes(n * elemSize)
}

func (r *reader) skipList() error {
	elemTag, err := r.byteValue()
	if err != nil {
		return err
	}
	n, err := r.arrayLen()
	if err != nil {
		return err
	}
	for range n {
		if err := r.skipPayload(elemTag); err != nil {
			return err
		}
	}
	return nil
}

// skipCompound は TAG_End までを読み飛ばす。
func (r *reader) skipCompound() error {
	for {
		tag, err := r.byteValue()
		if err != nil {
			return err
		}
		if tag == tagEnd {
			return nil
		}
		if _, err := r.stringValue(); err != nil {
			return err
		}
		if err := r.skipPayload(tag); err != nil {
			return err
		}
	}
}

// visitCompound は TAG_Compound の各要素に fn を適用する。
//
// fn が false を返した要素は読み飛ばす。true を返した場合、fn が値を
// 読み終えている必要がある。
func (r *reader) visitCompound(fn func(tag byte, name string) (handled bool, err error)) error {
	for {
		tag, err := r.byteValue()
		if err != nil {
			return err
		}
		if tag == tagEnd {
			return nil
		}
		name, err := r.stringValue()
		if err != nil {
			return err
		}
		handled, err := fn(tag, name)
		if err != nil {
			return err
		}
		if !handled {
			if err := r.skipPayload(tag); err != nil {
				return err
			}
		}
	}
}

// newReader は入力を全部読み込んで reader を作る。
// level.dat は実測 637 バイトなので、全体を持っても問題にならない。
func newReader(rd io.Reader) (*reader, error) {
	b, err := io.ReadAll(io.LimitReader(rd, maxPayloadLen+1))
	if err != nil {
		return nil, fmt.Errorf("NBT を読めません: %w", err)
	}
	if len(b) > maxPayloadLen {
		return nil, fmt.Errorf("level.dat が大きすぎます（%d バイト超）", maxPayloadLen)
	}
	if len(b) == 0 {
		return nil, errTruncated
	}
	return &reader{buf: b}, nil
}
