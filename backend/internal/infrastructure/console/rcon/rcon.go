// Package rcon はサーバーへコマンドを送る。
//
// 実装は docker compose exec -T mc rcon-cli を呼ぶだけの薄いラッパー。
// ホストに RCON クライアントを入れず、25575 も公開しないという既存の方針
// （README がハードルールとして定めている）をそのまま踏襲している。
// RCON は平文で総当たり保護も無いため、通信をコンテナ内に閉じ込める。
package rcon

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// Executor はコンテナ内でコマンドを実行する。compose.Runner が実装する。
type Executor interface {
	Exec(ctx context.Context, argv []string) (string, error)
}

// Client は rcon-cli 経由でサーバーを操作する。
type Client struct {
	exec Executor
}

// NewClient は Client を作る。
func NewClient(exec Executor) *Client { return &Client{exec: exec} }

// SaveOff はワールドの保存を止める。
//
// これを呼んだままにすると、以降の変更がディスクに書かれない。
// しかも症状が何も出ないため気づけない。必ず SaveOn と対で使う。
func (c *Client) SaveOff(ctx context.Context) error {
	return c.command(ctx, "save-off")
}

// SaveAll はワールドをディスクへ書き出す。
func (c *Client) SaveAll(ctx context.Context) error {
	return c.command(ctx, "save-all")
}

// SaveOn はワールドの保存を再開する。
//
// 冪等であることがこの設計の前提になっている。RCON には「保存が有効か」を
// 問い合わせる手段がないため、状態を照会せず無条件に再送する。
// 起動時と healthy への遷移時に毎回送っても問題にならない。
func (c *Client) SaveOn(ctx context.Context) error {
	return c.command(ctx, "save-on")
}

func (c *Client) command(ctx context.Context, cmd string) error {
	if _, err := c.exec.Exec(ctx, []string{"rcon-cli", cmd}); err != nil {
		return fmt.Errorf("rcon-cli %s に失敗しました: %w", cmd, err)
	}
	return nil
}

// listRE は list コマンドの出力から人数を取り出す。
//
// 出力書式はサーバーのバージョンに依存する。Paper 26.2 の実測は
// "There are 0 of a max of 5 players online:"。
// 将来変わりうるため、解釈できない場合を正常系として扱う。
var listRE = regexp.MustCompile(`There are (\d+) of a max of (\d+) players online`)

// ansiRE は端末の色コード。tty: true の副作用で混じることがある。
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// PlayerCount はオンライン人数と最大人数を返す。
//
// 出力を解釈できない場合は ok が偽になる。エラーにしないのは、
// 人数が読めないことで状態表示そのものを失敗させないため。
// 画面には「不明」と出せばよく、他の項目は問題なく表示できる。
func (c *Client) PlayerCount(ctx context.Context) (online, max int, ok bool, err error) {
	out, err := c.exec.Exec(ctx, []string{"rcon-cli", "list"})
	if err != nil {
		return 0, 0, false, fmt.Errorf("rcon-cli list に失敗しました: %w", err)
	}

	m := listRE.FindStringSubmatch(ansiRE.ReplaceAllString(strings.TrimSpace(out), ""))
	if m == nil {
		return 0, 0, false, nil
	}

	online, err = strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, false, nil
	}
	max, err = strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, false, nil
	}
	return online, max, true, nil
}

var _ port.ServerConsole = (*Client)(nil)
