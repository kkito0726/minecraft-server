package backup

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrInvalidRetentionPolicy は保持ポリシーとして使えない値であることを表す。
var ErrInvalidRetentionPolicy = errors.New("保持ポリシーが不正です")

// RetentionPolicy はバックアップをいくつ・いつまで残すか。
//
// keepCount が主、keepDays は従。keepDays は「世代数の枠から溢れたものに
// 与える猶予」として働き、単独では削除の引き金にならない。
//
//	keepCount=10, keepDays=7 → 新しい 10 件は無条件で残す。
//	                            11 件目以降は、7 日を過ぎたものだけ消す。
//
// この保守的な組み合わせ（両方が「消してよい」と言ったものだけ消す）にしてある。
// どちらか一方で消す方式にすると、日数の設定を短くしただけで
// 直近のバックアップまで消えてしまう。
//
// keepCount が 1 以上であることを型の不変条件にしている。
// 0 を許すと、適用した瞬間にすべてのバックアップが消える。
type RetentionPolicy struct {
	keepCount int
	keepDays  int
}

// NewRetentionPolicy は保持ポリシーを作る。
// keepDays が 0 なら日数による判定を行わない。
func NewRetentionPolicy(keepCount, keepDays int) (RetentionPolicy, error) {
	if keepCount < 1 {
		return RetentionPolicy{}, fmt.Errorf(
			"%w: 保持世代数が %d です。1 以上である必要があります", ErrInvalidRetentionPolicy, keepCount)
	}
	if keepDays < 0 {
		return RetentionPolicy{}, fmt.Errorf(
			"%w: 保持日数が %d です。0 以上である必要があります", ErrInvalidRetentionPolicy, keepDays)
	}
	return RetentionPolicy{keepCount: keepCount, keepDays: keepDays}, nil
}

// KeepCount は残す世代数を返す。
func (p RetentionPolicy) KeepCount() int { return p.keepCount }

// KeepDays は保持日数を返す。0 なら日数による判定を行わない。
func (p RetentionPolicy) KeepDays() int { return p.keepDays }

// RetentionEntry は保持判定に必要な情報。
type RetentionEntry struct {
	ID        ID
	CreatedAt time.Time
}

// SelectForDeletion は削除すべきバックアップを選ぶ。
//
// 世代数の枠から溢れ、かつ猶予期間を過ぎたものだけを削除する。
// どちらか一方でも「残す」と言っているなら残す。
//
// 設定がどうであれ、最も新しい 1 件は必ず残す。これが無いと、
// 保持日数を短く設定しただけでバックアップが全損する。
//
// 入力は変更しない。
func SelectForDeletion(entries []RetentionEntry, policy RetentionPolicy, now time.Time) []ID {
	if len(entries) <= 1 {
		return nil
	}

	// 新しい順に並べた複製で判定する。呼び出し側の並び順に依存しない。
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b RetentionEntry) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})

	var deleted []ID
	for i, e := range sorted {
		// 最も新しい 1 件は無条件で残す
		if i == 0 {
			continue
		}
		if exceedsCount(i, policy) && gracePeriodExpired(e.CreatedAt, policy, now) {
			deleted = append(deleted, e.ID)
		}
	}
	return deleted
}

// exceedsCount は、新しい順で index 番目のものが世代数の枠から溢れているかを返す。
func exceedsCount(index int, policy RetentionPolicy) bool {
	return index >= policy.keepCount
}

// gracePeriodExpired は猶予期間を過ぎているかを返す。
// 保持日数が 0 のときは猶予を設けないため、常に「消してよい」を返す
// （世代数の判定だけで決まる）。
func gracePeriodExpired(createdAt time.Time, policy RetentionPolicy, now time.Time) bool {
	if policy.keepDays == 0 {
		return true
	}
	limit := time.Duration(policy.keepDays) * 24 * time.Hour
	return now.Sub(createdAt) > limit
}
