package rcon_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/console/rcon"
)

// fakeExec は compose.Runner.Exec の代わり。呼び出しを記録する。
type fakeExec struct {
	calls   [][]string
	outputs map[string]string
	err     error
}

func (f *fakeExec) Exec(_ context.Context, argv []string) (string, error) {
	f.calls = append(f.calls, argv)
	if f.err != nil {
		return "", f.err
	}
	if out, ok := f.outputs[strings.Join(argv, " ")]; ok {
		return out, nil
	}
	return "", nil
}

func (f *fakeExec) commands() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}

// RCON は rcon-cli 経由で叩く。25575 は公開していないため、
// コンテナの外からネットワークで繋ぐ経路は存在しない。
func TestSaveCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*rcon.Client) error
		want string
	}{
		{"save-off", func(c *rcon.Client) error { return c.SaveOff(context.Background()) }, "rcon-cli save-off"},
		{"save-all", func(c *rcon.Client) error { return c.SaveAll(context.Background()) }, "rcon-cli save-all"},
		{"save-on", func(c *rcon.Client) error { return c.SaveOn(context.Background()) }, "rcon-cli save-on"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exec := &fakeExec{}
			c := rcon.NewClient(exec)

			if err := tt.call(c); err != nil {
				t.Fatalf("失敗: %v", err)
			}
			if got := exec.commands(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("実行されたコマンドが %v。%q のはず", got, tt.want)
			}
		})
	}
}

// list の出力書式はサーバーのバージョンに依存する。
// 解釈できない場合は人数不明として扱い、状態取得そのものを失敗させない。
func TestPlayerCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		output     string
		wantOnline int
		wantMax    int
		wantOK     bool
	}{
		{
			name:       "Paper 26.2 の実際の出力（誰もいない）",
			output:     "There are 0 of a max of 5 players online:",
			wantOnline: 0, wantMax: 5, wantOK: true,
		},
		{
			name:       "プレイヤーがいる",
			output:     "There are 2 of a max of 5 players online: Alice, Bob",
			wantOnline: 2, wantMax: 5, wantOK: true,
		},
		{
			name:       "末尾に改行がある",
			output:     "There are 1 of a max of 20 players online: Alice\n",
			wantOnline: 1, wantMax: 20, wantOK: true,
		},
		{
			name:       "色コードが混じる",
			output:     "\x1b[0mThere are 3 of a max of 10 players online:\x1b[0m",
			wantOnline: 3, wantMax: 10, wantOK: true,
		},
		{
			name:   "解釈できない書式",
			output: "Unknown command. Type \"/help\" for help.",
			wantOK: false,
		},
		{
			name:   "空の出力",
			output: "",
			wantOK: false,
		},
		{
			name:   "数字が無い",
			output: "There are some of a max of many players online:",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exec := &fakeExec{outputs: map[string]string{"rcon-cli list": tt.output}}
			c := rcon.NewClient(exec)

			online, max, ok, err := c.PlayerCount(context.Background())
			if err != nil {
				t.Fatalf("エラーにせず不明として扱うはず: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok が %v。%v のはず", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if online != tt.wantOnline || max != tt.wantMax {
				t.Errorf("人数が %d/%d。%d/%d のはず", online, max, tt.wantOnline, tt.wantMax)
			}
		})
	}
}

// コンテナが停止していると Exec 自体が失敗する。
// その場合は「不明」ではなくエラーとして返し、呼び出し側が判断する。
func TestPlayerCountWhenExecFails(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{err: errors.New("コンテナが見つかりません")}
	c := rcon.NewClient(exec)

	if _, _, _, err := c.PlayerCount(context.Background()); err == nil {
		t.Error("エラーになるはず")
	}
}

// save-on は冪等。RCON には保存が有効かを問い合わせる手段がないため、
// 状態を照会せず無条件に再送する設計になっている。
// 何度呼んでも壊れないことを前提にしている。
func TestSaveOnIsRepeatable(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{}
	c := rcon.NewClient(exec)

	for range 3 {
		if err := c.SaveOn(context.Background()); err != nil {
			t.Fatalf("失敗: %v", err)
		}
	}
	if got := exec.commands(); len(got) != 3 {
		t.Errorf("呼び出しが %d 回。3 回のはず", len(got))
	}
}

func TestCommandFailurePropagates(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{err: errors.New("実行に失敗")}
	c := rcon.NewClient(exec)

	if err := c.SaveOff(context.Background()); err == nil {
		t.Error("エラーが伝わるはず")
	}
}
