package operation_test

import (
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// 種類の名前は進捗バナーにそのまま出る。空にならないこと。
func TestKindString(t *testing.T) {
	t.Parallel()

	kinds := []operation.Kind{
		operation.KindServerStart, operation.KindServerStop, operation.KindServerRestart,
		operation.KindBackupCreate, operation.KindBackupRestore,
		operation.KindWorldSwitch, operation.KindWorldCreate, operation.KindWorldClone,
		operation.KindWorldRename, operation.KindWorldDelete,
		operation.KindUnspecified, operation.Kind(99),
	}

	seen := map[string]operation.Kind{}
	for _, k := range kinds {
		name := k.String()
		if name == "" {
			t.Errorf("%d の String() が空", k)
			continue
		}
		// 未定義以外は名前が重複しないこと。同じ表示だと
		// どの操作が走っているか分からない。
		if k != operation.KindUnspecified && k != operation.Kind(99) {
			if prev, dup := seen[name]; dup {
				t.Errorf("%d と %d の表示が同じ: %q", prev, k, name)
			}
			seen[name] = k
		}
	}
}

// 終端の判定を誤ると、完了した操作が実行中のままになり
// UI の排他表示が永久に解けなくなる。
func TestStateIsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state operation.State
		want  bool
	}{
		{operation.StatePending, false},
		{operation.StateRunning, false},
		{operation.StateSucceeded, true},
		{operation.StateFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Errorf("IsTerminal() = %v。%v のはず", got, tt.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	t.Parallel()

	states := []operation.State{
		operation.StatePending, operation.StateRunning,
		operation.StateSucceeded, operation.StateFailed, operation.State(99),
	}
	for _, s := range states {
		if s.String() == "" {
			t.Errorf("%d の String() が空", s)
		}
	}
}

func TestNewID(t *testing.T) {
	t.Parallel()

	if _, err := operation.NewID("op-1"); err != nil {
		t.Errorf("受理されるはずが %v", err)
	}
	for _, s := range []string{"", "   ", "\t"} {
		if _, err := operation.NewID(s); err == nil {
			t.Errorf("%q は拒否されるはず", s)
		}
	}

	var zero operation.ID
	if zero.IsValid() {
		t.Error("ゼロ値が有効になっている")
	}
	if zero.String() != "" {
		t.Errorf("ゼロ値の String() が %q", zero.String())
	}
}

func TestMustIDPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("panic するはず")
		}
	}()
	operation.MustID("")
}
