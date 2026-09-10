// Package leveldat はワールドの level.dat からバージョン情報を読む。
//
// バージョン判定に使うのは Data.DataVersion（整数）であって
// Data.Version.Name（表示文字列）ではない。表示文字列は同じ "26.2" でも
// スナップショット間で DataVersion が異なることがあり、
// ワールドのアップグレードは片道なので取り違えると戻せない。
//
// NBT の読み取りは自前で実装している。取り出すのは 6 フィールドだけで、
// Raspberry Pi 5 (4GB) 上で MC が既に 2.7GB 使う環境に、Minecraft の
// プロトコル全体を扱うライブラリを持ち込む理由がないため。
// 差し替えたくなった場合に備えて Reader インターフェースの背後に隠してある。
package leveldat

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// ErrNotLevelDat は NBT としては読めたが level.dat の構造ではないことを表す。
var ErrNotLevelDat = errors.New("level.dat の構造ではありません（Data が見つかりません）")

// Info は level.dat から読み取った情報。
//
// 読めなかったフィールドはゼロ値になる。level.dat の構造はバージョンで
// 変わるため、全部そろっていることを前提にしない。
type Info struct {
	// Data.LevelName。ディレクトリ名と一致しないことがある。
	LevelName string
	// Data.Version.Name。表示専用（例 "26.2"）。
	VersionName string
	// Data.DataVersion。バージョン比較のキーはこれ（例 4903）。
	DataVersion int32
	// Data.Version.Snapshot。
	Snapshot bool
	// Data.LastPlayed。ミリ秒エポック。
	LastPlayed time.Time
	// Data.Bukkit.Version。Paper のビルド情報。
	ServerBrand string
}

// Reader は level.dat を読む。差し替えられるようにインターフェースにしてある。
type Reader interface {
	Read(r io.Reader) (Info, error)
}

// NBTReader は自前の NBT 実装で読む Reader。
type NBTReader struct{}

// Read は level.dat を読む。
func (NBTReader) Read(r io.Reader) (Info, error) { return Read(r) }

var _ Reader = NBTReader{}

// Read は level.dat を読む。gzip / zlib / 非圧縮を自動で判別する。
func Read(r io.Reader) (Info, error) {
	decompressed, err := decompress(r)
	if err != nil {
		return Info{}, err
	}

	nbt, err := newReader(decompressed)
	if err != nil {
		return Info{}, err
	}
	return parseRoot(nbt)
}

// ReadFile はパスを指定して level.dat を読む。
func ReadFile(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, fmt.Errorf("level.dat を開けません (%s): %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := Read(f)
	if err != nil {
		return Info{}, fmt.Errorf("level.dat を読めません (%s): %w", path, err)
	}
	return info, nil
}

// decompress は先頭のマジックバイトで圧縮形式を判別して展開する。
func decompress(r io.Reader) (io.Reader, error) {
	head, err := io.ReadAll(io.LimitReader(r, maxPayloadLen+1))
	if err != nil {
		return nil, fmt.Errorf("level.dat を読めません: %w", err)
	}
	if len(head) < 2 {
		return nil, errTruncated
	}

	switch {
	case head[0] == 0x1f && head[1] == 0x8b:
		return wrap(gzip.NewReader(bytes.NewReader(head)))
	// zlib は 0x78 で始まる。level.dat では稀だがチャンクデータで使われる形式。
	case head[0] == 0x78:
		return wrap(zlib.NewReader(bytes.NewReader(head)))
	// 非圧縮の NBT はルートの TAG_Compound から始まる。
	case head[0] == tagCompound:
		return bytes.NewReader(head), nil
	default:
		return nil, fmt.Errorf("level.dat の形式を判別できません（先頭バイト 0x%02x）", head[0])
	}
}

func wrap(rc io.ReadCloser, err error) (io.Reader, error) {
	if err != nil {
		return nil, fmt.Errorf("level.dat を展開できません: %w", err)
	}
	defer func() { _ = rc.Close() }()

	b, err := io.ReadAll(io.LimitReader(rc, maxPayloadLen+1))
	if err != nil {
		return nil, fmt.Errorf("level.dat を展開できません: %w", err)
	}
	return bytes.NewReader(b), nil
}

// parseRoot はルートの TAG_Compound から Data を探して読む。
func parseRoot(r *reader) (Info, error) {
	tag, err := r.byteValue()
	if err != nil {
		return Info{}, err
	}
	if tag != tagCompound {
		return Info{}, fmt.Errorf("%w（ルートが TAG_Compound ではありません）", ErrNotLevelDat)
	}
	if _, err := r.stringValue(); err != nil {
		return Info{}, err
	}

	var (
		info  Info
		found bool
	)
	err = r.visitCompound(func(tag byte, name string) (bool, error) {
		if tag != tagCompound || name != "Data" {
			return false, nil
		}
		found = true
		return true, parseData(r, &info)
	})
	if err != nil {
		return Info{}, err
	}
	if !found {
		return Info{}, ErrNotLevelDat
	}
	return info, nil
}

// parseData は Data コンパウンドから必要なフィールドを拾う。
func parseData(r *reader, info *Info) error {
	return r.visitCompound(func(tag byte, name string) (bool, error) {
		switch {
		case tag == tagString && name == "LevelName":
			return true, assignString(r, &info.LevelName)
		case tag == tagInt && name == "DataVersion":
			return true, assignInt32(r, &info.DataVersion)
		case tag == tagLong && name == "LastPlayed":
			return true, assignMillis(r, &info.LastPlayed)
		case tag == tagCompound && name == "Version":
			return true, parseVersion(r, info)
		case tag == tagCompound && name == "Bukkit.Version":
			// 名前は Compound だが実体は文字列のこともある。読み飛ばす。
			return false, nil
		case tag == tagString && name == "Bukkit.Version":
			return true, assignString(r, &info.ServerBrand)
		default:
			return false, nil
		}
	})
}

// parseVersion は Data.Version から Name と Snapshot を拾う。
//
// DataVersion は Data 直下にも Version.Id にもあるが、Data 直下を正とする。
// Version.Id は Data 直下が読めなかった場合の補完に使う。
func parseVersion(r *reader, info *Info) error {
	return r.visitCompound(func(tag byte, name string) (bool, error) {
		switch {
		case tag == tagString && name == "Name":
			return true, assignString(r, &info.VersionName)
		case tag == tagByte && name == "Snapshot":
			b, err := r.byteValue()
			if err != nil {
				return true, err
			}
			info.Snapshot = b != 0
			return true, nil
		case tag == tagInt && name == "Id":
			v, err := r.int32Value()
			if err != nil {
				return true, err
			}
			if info.DataVersion == 0 {
				info.DataVersion = v
			}
			return true, nil
		default:
			return false, nil
		}
	})
}

func assignString(r *reader, dst *string) error {
	v, err := r.stringValue()
	if err != nil {
		return err
	}
	*dst = v
	return nil
}

func assignInt32(r *reader, dst *int32) error {
	v, err := r.int32Value()
	if err != nil {
		return err
	}
	*dst = v
	return nil
}

// assignMillis はミリ秒エポックを time.Time にする。
// 秒として解釈すると 1970 年付近になり、最終プレイ日時の表示が壊れる。
func assignMillis(r *reader, dst *time.Time) error {
	v, err := r.int64Value()
	if err != nil {
		return err
	}
	if v > 0 {
		*dst = time.UnixMilli(v)
	}
	return nil
}
