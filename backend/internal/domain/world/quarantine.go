package world

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrActiveWorld は稼働中のワールドに対する操作が拒否されたことを表す。
var ErrActiveWorld = errors.New("稼働中のワールドです。先に別のワールドへ切り替えてください")

// ErrInvalidQuarantine は退避ディレクトリの名前として解釈できないことを表す。
var ErrInvalidQuarantine = errors.New("退避ディレクトリの名前ではありません")

// ErrConfirmationMismatch は削除の確認入力が対象と一致しないことを表す。
//
// 打ち間違いは利用者が自分で直せる。内部エラーに丸めず、
// 何が違うのかをそのまま伝える。
var ErrConfirmationMismatch = errors.New("確認の名前が一致していません")

// ErrNotFound は対象が見つからないことを表す。
var ErrNotFound = errors.New("見つかりません")

// ErrAlreadyExists は同じ名前のものが既にあることを表す。
var ErrAlreadyExists = errors.New("同じ名前のワールドが既にあります")

// QuarantineKind は退避の理由。
type QuarantineKind int

const (
	// QuarantineFromRestore は復元によって退避されたことを表す。
	QuarantineFromRestore QuarantineKind = iota
	// QuarantineFromDelete は削除によって退避されたことを表す。
	QuarantineFromDelete
)

// marker は退避ディレクトリ名に付ける目印。
func (k QuarantineKind) marker() string {
	if k == QuarantineFromDelete {
		return ".deleted-"
	}
	return ".broken-"
}

// String は理由の名前を返す。
func (k QuarantineKind) String() string {
	if k == QuarantineFromDelete {
		return "削除"
	}
	return "復元"
}

// quarantineTimeLayout は退避ディレクトリ名の日時部分の書式。
// docs/backup-restore.md の手順（date +%Y%m%d-%H%M%S）に合わせている。
const quarantineTimeLayout = "20060102-150405"

// Quarantine は退避されたワールドディレクトリ。
//
// 復元も削除も、元のディレクトリを消さずにここへ移動する。
// 失敗しても戻せるようにするための不変条件であり、
// システムはこれを自動削除しない。
type Quarantine struct {
	dirName      string
	original     Name
	kind         QuarantineKind
	quarantinedA time.Time
	sizeBytes    int64
}

// NewQuarantineName は退避先のディレクトリ名を組み立てる。
func NewQuarantineName(original Name, kind QuarantineKind, at time.Time) string {
	return original.String() + kind.marker() + at.Format(quarantineTimeLayout)
}

// ParseQuarantine は退避ディレクトリ名を解釈する。
func ParseQuarantine(dirName string, sizeBytes int64) (Quarantine, error) {
	for _, kind := range []QuarantineKind{QuarantineFromRestore, QuarantineFromDelete} {
		base, ts, ok := strings.Cut(dirName, kind.marker())
		if !ok {
			continue
		}
		original, err := NewName(base)
		if err != nil {
			return Quarantine{}, fmt.Errorf("%w: 元のワールド名が不正です (%s)", ErrInvalidQuarantine, dirName)
		}
		at, err := time.ParseInLocation(quarantineTimeLayout, ts, time.Local)
		if err != nil {
			return Quarantine{}, fmt.Errorf("%w: 日時を解釈できません (%s)", ErrInvalidQuarantine, dirName)
		}
		return Quarantine{
			dirName:      dirName,
			original:     original,
			kind:         kind,
			quarantinedA: at,
			sizeBytes:    sizeBytes,
		}, nil
	}
	return Quarantine{}, fmt.Errorf("%w: %s", ErrInvalidQuarantine, dirName)
}

// DirName はディレクトリ名を返す。
func (q Quarantine) DirName() string { return q.dirName }

// OriginalName は退避元のワールド名を返す。
func (q Quarantine) OriginalName() Name { return q.original }

// Kind は退避の理由を返す。
func (q Quarantine) Kind() QuarantineKind { return q.kind }

// QuarantinedAt は退避した日時を返す。
func (q Quarantine) QuarantinedAt() time.Time { return q.quarantinedA }

// SizeBytes はディレクトリのサイズを返す。
func (q Quarantine) SizeBytes() int64 { return q.sizeBytes }

// IsValid は有効な Quarantine かを返す。ゼロ値は無効。
func (q Quarantine) IsValid() bool { return q.dirName != "" }
