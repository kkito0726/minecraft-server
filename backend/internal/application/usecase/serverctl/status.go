// Package serverctl はサーバーの状態取得と起動・停止のユースケース。
package serverctl

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// 設定のキー。.env から読む。
const (
	keyVersion = "MC_VERSION"
	keyLevel   = "MC_LEVEL"
)

// unknownPlayerCount は人数を解釈できなかったことを表す。
//
// 0 で代用すると「誰もいない」と区別できない。
const unknownPlayerCount = -1

// Status はサーバーの現在の状態一式。
type Status struct {
	Container          server.ContainerStatus
	ConfiguredVersion  string
	ActiveLevel        world.Name
	ActiveWorldVersion shared.WorldVersion
	// OnlinePlayers と MaxPlayers は解釈できなければ -1。
	OnlinePlayers int
	MaxPlayers    int
	SavingState   server.SavingState
	// InterruptedDetected は前回の実行が操作の途中で終了していたか。
	InterruptedDetected bool
}

// StatusConfig は StatusUseCase の依存。
type StatusConfig struct {
	Runtime port.ContainerRuntime
	Console port.ServerConsole
	Config  port.ServerConfig
	Levels  port.LevelReader
}

// StatusUseCase は状態の取得。
type StatusUseCase struct {
	cfg StatusConfig
	// interrupted は起動時の回復処理が中断を検出したかどうか。
	// 画面に警告を出し続けるために保持する。
	interrupted atomic.Bool
}

// NewStatusUseCase は StatusUseCase を作る。
func NewStatusUseCase(cfg StatusConfig) (*StatusUseCase, error) {
	if cfg.Runtime == nil || cfg.Console == nil || cfg.Config == nil {
		return nil, errors.New("runtime / console / config が必要です")
	}
	return &StatusUseCase{cfg: cfg}, nil
}

// SetInterrupted は中断の検出結果を記録する。
func (u *StatusUseCase) SetInterrupted(v bool) { u.interrupted.Store(v) }

// Execute は現在の状態を集める。
//
// 個々の取得が失敗しても、状態そのものは返す。状態表示は画面の入口であり、
// ここで失敗すると管理コンソール全体が使えなくなる。
func (u *StatusUseCase) Execute(ctx context.Context) (Status, error) {
	container, err := u.cfg.Runtime.Status(ctx)
	if err != nil {
		return Status{}, err
	}

	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return Status{}, err
	}

	version, _ := snapshot.Get(keyVersion)
	level := u.activeLevel(snapshot)

	status := Status{
		Container:           container,
		ConfiguredVersion:   version,
		ActiveLevel:         level,
		ActiveWorldVersion:  u.worldVersion(ctx, level),
		OnlinePlayers:       unknownPlayerCount,
		MaxPlayers:          unknownPlayerCount,
		SavingState:         u.savingState(),
		InterruptedDetected: u.interrupted.Load(),
	}

	// 停止中に RCON を叩いても失敗するだけなので省略する。
	if container.State.IsUp() {
		status.OnlinePlayers, status.MaxPlayers = u.playerCount(ctx)
	}
	return status, nil
}

func (u *StatusUseCase) activeLevel(snapshot port.ConfigSnapshot) world.Name {
	value, ok := snapshot.Get(keyLevel)
	if !ok || value == "" {
		value = "world"
	}
	// .env に不正な名前が手で書かれている可能性がある。
	// 読めなければゼロ値のまま返し、画面には空として出す。
	n, err := world.NewName(value)
	if err != nil {
		return world.Name{}
	}
	return n
}

func (u *StatusUseCase) worldVersion(ctx context.Context, level world.Name) shared.WorldVersion {
	if u.cfg.Levels == nil || !level.IsValid() {
		return shared.UnreadableWorldVersion()
	}
	return u.cfg.Levels.ReadWorld(ctx, level)
}

// playerCount は人数を取る。取れなければ -1 を返す。
//
// エラーにしないのは、人数が読めないことで状態表示そのものを
// 失敗させないため。画面には「不明」と出せばよい。
func (u *StatusUseCase) playerCount(ctx context.Context) (online, max int) {
	o, m, ok, err := u.cfg.Console.PlayerCount(ctx)
	if err != nil || !ok {
		return unknownPlayerCount, unknownPlayerCount
	}
	return o, m
}

// savingState は保存が有効かの推定値を返す。
//
// RCON には問い合わせる手段がないため実測できない。中断が検出されて
// いれば「停止している可能性あり」として警告する。
func (u *StatusUseCase) savingState() server.SavingState {
	if u.interrupted.Load() {
		return server.SavingSuspectOff
	}
	return server.SavingAssumedOn
}
