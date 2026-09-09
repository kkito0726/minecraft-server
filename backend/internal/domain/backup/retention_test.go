package backup_test

import (
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

// 保持世代数が 0 以下だと、適用した瞬間に全部消える。
// 型として作れないようにする。
func TestNewRetentionPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keepCount int
		keepDays  int
		wantErr   bool
	}{
		{"既定値", 10, 0, false},
		{"最小の世代数", 1, 0, false},
		{"日数も指定", 10, 7, false},
		{"世代数 0 は不正", 0, 0, true},
		{"世代数が負は不正", -1, 0, true},
		{"日数が負は不正", 10, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := backup.NewRetentionPolicy(tt.keepCount, tt.keepDays)
			if tt.wantErr {
				if err == nil {
					t.Error("エラーになるはず")
				}
				return
			}
			if err != nil {
				t.Fatalf("受理されるはずが %v", err)
			}
			if p.KeepCount() != tt.keepCount || p.KeepDays() != tt.keepDays {
				t.Errorf("値が %d/%d", p.KeepCount(), p.KeepDays())
			}
		})
	}
}

// entry は保持判定の対象。実際の Backup から必要な情報だけを取り出したもの。
func entries(t *testing.T, ages ...time.Duration) []backup.RetentionEntry {
	t.Helper()

	now := time.Now()
	out := make([]backup.RetentionEntry, len(ages))
	for i, age := range ages {
		id, err := backup.NewID(idFor(i))
		if err != nil {
			t.Fatal(err)
		}
		out[i] = backup.RetentionEntry{ID: id, CreatedAt: now.Add(-age)}
	}
	return out
}

func idFor(i int) string {
	return "backup-26.2-world-2026090" + string(rune('0'+i)) + "-000000.zip"
}

func TestSelectForDeletionByCount(t *testing.T) {
	t.Parallel()

	// 新しい順に 0 日前, 1 日前, ... 4 日前 の 5 件
	list := entries(t, 0, day(1), day(2), day(3), day(4))
	policy := mustPolicy(t, 3, 0)

	deleted := backup.SelectForDeletion(list, policy, time.Now())

	if len(deleted) != 2 {
		t.Fatalf("削除対象が %d 件。2 件のはず", len(deleted))
	}
	// 最も古い 2 件が対象
	for _, id := range deleted {
		if id == list[0].ID || id == list[1].ID || id == list[2].ID {
			t.Errorf("新しい方の %s が削除対象になっている", id)
		}
	}
}

// 日数は世代数の枠から溢れたものに対する猶予として働く。
// 単独では削除の引き金にならない（保守的な AND）。
func TestSelectForDeletionDaysActAsGracePeriod(t *testing.T) {
	t.Parallel()

	now := time.Now()
	list := entries(t, 0, day(3), day(10), day(20))

	t.Run("世代数の枠に収まっていれば、古くても消さない", func(t *testing.T) {
		t.Parallel()
		// 4 件すべてが世代数 100 の枠内なので、20 日前のものも残る
		if got := backup.SelectForDeletion(list, mustPolicy(t, 100, 7), now); len(got) != 0 {
			t.Errorf("削除対象が %d 件。世代数の枠内なので 0 件のはず", len(got))
		}
	})

	t.Run("枠から溢れ、かつ猶予も過ぎたものだけ消す", func(t *testing.T) {
		t.Parallel()
		// 世代数 2 → 3・4 番目（10 日前・20 日前）が枠外。どちらも 7 日を超えている
		if got := backup.SelectForDeletion(list, mustPolicy(t, 2, 7), now); len(got) != 2 {
			t.Errorf("削除対象が %d 件。2 件のはず", len(got))
		}
	})

	t.Run("枠から溢れても猶予内なら残す", func(t *testing.T) {
		t.Parallel()
		// 世代数 2 → 3・4 番目が枠外だが、猶予 30 日を超えていない
		if got := backup.SelectForDeletion(list, mustPolicy(t, 2, 30), now); len(got) != 0 {
			t.Errorf("削除対象が %d 件。猶予内なので 0 件のはず", len(got))
		}
	})
}

// 日数が 0 なら猶予は無効。世代数だけで判定する。
func TestSelectForDeletionWithoutDays(t *testing.T) {
	t.Parallel()

	list := entries(t, 0, day(10), day(20))

	if got := backup.SelectForDeletion(list, mustPolicy(t, 1, 0), time.Now()); len(got) != 2 {
		t.Errorf("削除対象が %d 件。2 件のはず", len(got))
	}
}

// 設定がどうであれ、最も新しい 1 件は必ず残す。
// これが無いと保持日数の誤設定でバックアップが全損する。
func TestSelectForDeletionAlwaysKeepsNewest(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// 全部 1 年前
	list := entries(t, day(365), day(366), day(367))
	policy := mustPolicy(t, 1, 1)

	deleted := backup.SelectForDeletion(list, policy, now)

	if len(deleted) != 2 {
		t.Fatalf("削除対象が %d 件。最新 1 件を残して 2 件のはず", len(deleted))
	}
	newest := newestOf(list)
	for _, id := range deleted {
		if id == newest {
			t.Error("最も新しいバックアップが削除対象になっている")
		}
	}
}

func TestSelectForDeletionEdgeCases(t *testing.T) {
	t.Parallel()

	now := time.Now()
	policy := mustPolicy(t, 3, 7)

	t.Run("空の一覧", func(t *testing.T) {
		t.Parallel()
		if got := backup.SelectForDeletion(nil, policy, now); len(got) != 0 {
			t.Errorf("削除対象が %d 件", len(got))
		}
	})

	t.Run("1 件だけ（古くても残す）", func(t *testing.T) {
		t.Parallel()
		list := entries(t, day(365))
		if got := backup.SelectForDeletion(list, policy, now); len(got) != 0 {
			t.Errorf("削除対象が %d 件。1 件だけなら残すはず", len(got))
		}
	})

	t.Run("保持世代数ちょうど", func(t *testing.T) {
		t.Parallel()
		list := entries(t, 0, day(1), day(2))
		if got := backup.SelectForDeletion(list, policy, now); len(got) != 0 {
			t.Errorf("削除対象が %d 件", len(got))
		}
	})

	t.Run("枠外だが猶予内", func(t *testing.T) {
		t.Parallel()
		// 世代数 3 の枠外に 1 件あるが、まだ 7 日経っていない
		list := entries(t, 0, day(1), day(2), day(3))
		if got := backup.SelectForDeletion(list, policy, now); len(got) != 0 {
			t.Errorf("削除対象が %d 件。猶予内なので 0 件のはず", len(got))
		}
	})
}

// 入力の順序に依存しない。呼び出し側が並べ替えを忘れても正しく動くこと。
func TestSelectForDeletionIsOrderIndependent(t *testing.T) {
	t.Parallel()

	now := time.Now()
	policy := mustPolicy(t, 2, 0)

	ascending := entries(t, day(4), day(3), day(2), day(1), 0)
	descending := entries(t, 0, day(1), day(2), day(3), day(4))

	if a, d := len(backup.SelectForDeletion(ascending, policy, now)),
		len(backup.SelectForDeletion(descending, policy, now)); a != d {
		t.Errorf("順序で結果が変わった: %d 件 vs %d 件", a, d)
	}
}

// SelectForDeletion は入力を変更しない（イミュータブル）。
func TestSelectForDeletionDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	list := entries(t, 0, day(10), day(20))
	before := make([]backup.RetentionEntry, len(list))
	copy(before, list)

	backup.SelectForDeletion(list, mustPolicy(t, 1, 0), time.Now())

	for i := range list {
		if list[i].ID != before[i].ID {
			t.Errorf("%d 番目の要素が入れ替わっている", i)
		}
	}
}

func day(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

func mustPolicy(t *testing.T, count, days int) backup.RetentionPolicy {
	t.Helper()

	p, err := backup.NewRetentionPolicy(count, days)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newestOf(list []backup.RetentionEntry) backup.ID {
	newest := list[0]
	for _, e := range list[1:] {
		if e.CreatedAt.After(newest.CreatedAt) {
			newest = e
		}
	}
	return newest.ID
}
