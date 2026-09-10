// Package port はユースケースが外部に求める振る舞いをインターフェースで表す。
//
// 実装は infrastructure に置く。ユースケースは実装を知らないため、
// テストは呼び出しを記録する偽物を差し込むだけでよく、
// 実際の docker も実際の Minecraft サーバーも必要にならない。
//
// この層も外部を知らない。connect も net/http も os/exec も import しない。
package port

import (
	"context"
	"io"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// LogSink は外部コマンドの出力を 1 行ずつ受け取る。進捗ログに流す。
type LogSink func(line string)

// ContainerRuntime は docker compose の操作。
//
// RCON のポート 25575 は公開していないため、コンテナへの指示はすべてここを通る。
type ContainerRuntime interface {
	// Up はコンテナを作り直して起動する。
	// .env の変更は再作成でしか反映されないため、restart は用意しない。
	Up(ctx context.Context, sink LogSink) error
	// Down はコンテナを停止して削除する。stop_grace_period を尊重する。
	Down(ctx context.Context, sink LogSink) error
	// Status は現在の状態を返す。
	Status(ctx context.Context) (server.ContainerStatus, error)
	// WaitStopped はコンテナが存在しなくなるまで待つ。
	WaitStopped(ctx context.Context, timeout time.Duration) error
	// WaitReady はサーバーの起動完了を待つ。
	// ログの "Done (" とヘルスチェックの合格のいずれかで判定する。
	WaitReady(ctx context.Context, timeout time.Duration) error
}

// ServerConsole はサーバーへのコマンド送信。
//
// 実装は docker compose exec -T mc rcon-cli を呼ぶ。
type ServerConsole interface {
	// SaveOff はワールドの保存を止める。
	SaveOff(ctx context.Context) error
	// SaveAll はワールドを保存する。
	SaveAll(ctx context.Context) error
	// SaveOn はワールドの保存を再開する。冪等。
	//
	// 冪等であることが設計の前提になっている。RCON には保存が有効かを
	// 問い合わせる手段がないため、状態を照会せず無条件に再送する。
	SaveOn(ctx context.Context) error
	// PlayerCount はオンライン人数と最大人数を返す。
	// 出力を解釈できない場合は ok が偽になる。人数不明として扱い、
	// 状態取得そのものを失敗させない。
	PlayerCount(ctx context.Context) (online, max int, ok bool, err error)
}

// ServerConfig は .env の読み書き。
//
// Snapshot を介するのは、読み込みから書き込みまでの間に人間が
// vi .env で編集していないことを保証するため。
type ServerConfig interface {
	Load(ctx context.Context) (ConfigSnapshot, error)
	Save(ctx context.Context, snapshot ConfigSnapshot) error
}

// ConfigSnapshot は読み込んだ時点の .env。不変。
type ConfigSnapshot interface {
	// Get はキーの値を返す。
	Get(key string) (string, bool)
	// With はキーの値を差し替えた新しい Snapshot を返す。
	With(key, value string) ConfigSnapshot
}

// WorldRepository はワールドディレクトリの読み書き。
type WorldRepository interface {
	List(ctx context.Context) ([]world.World, error)
	Exists(ctx context.Context, name world.Name) (bool, error)
	// Copy は session.lock を除いて複製する。
	Copy(ctx context.Context, src, dst world.Name, progress Progress) error
	Rename(ctx context.Context, from, to world.Name) error
	// Quarantine はディレクトリを退避先へ移動する。削除はしない。
	// 復元に失敗しても戻せるようにするための不変条件。
	Quarantine(ctx context.Context, name world.Name, kind world.QuarantineKind) (world.Quarantine, error)
	// Restore は退避したディレクトリを元の位置へ戻す。
	Restore(ctx context.Context, q world.Quarantine, to world.Name) error
	Remove(ctx context.Context, name world.Name) error
	ListQuarantines(ctx context.Context) ([]world.Quarantine, error)
	RemoveQuarantine(ctx context.Context, q world.Quarantine) (freedBytes int64, err error)
	// AvailableBytes は data/ の空き容量を返す。
	AvailableBytes(ctx context.Context) (int64, error)
}

// Progress は進捗の通知。総量が不明な場合 total は 0。
type Progress func(done, total int64)

// LevelReader は level.dat からバージョンを読む。
//
// 読めなかった場合はエラーではなく「読めない」を表す WorldVersion を返す。
// 破損したワールドがあっても一覧表示そのものは継続する。
type LevelReader interface {
	ReadWorld(ctx context.Context, name world.Name) shared.WorldVersion
	Read(ctx context.Context, r io.Reader) shared.WorldVersion
}

// Clock は現在時刻。テストで固定するために抽象化する。
type Clock interface {
	Now() time.Time
}
