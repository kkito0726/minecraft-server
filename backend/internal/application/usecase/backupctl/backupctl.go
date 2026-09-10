// Package backupctl はバックアップの取得・一覧・削除・世代管理。
//
// 取得の中核は「save-on を必ず戻す」という 1 点にある。save-off を
// 送ったまま終わると、以降の変更がディスクに書かれないのに症状が
// 何も出ない。次の再起動で数時間分のプレイが消えて初めて気づく。
// そのため save-on は defer で戻し、加えてロックに記録を残して
// プロセスが落ちた場合の復旧経路（reconcile）も用意している。
package backupctl

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// 設定のキー。
const (
	keyLevel    = "MC_LEVEL"
	keyVersion  = "MC_VERSION"
	keyKeep     = "ADMIN_BACKUP_KEEP"
	keyKeepDays = "ADMIN_BACKUP_KEEP_DAYS"
)

// defaultLevel は MC_LEVEL が未設定のときのワールド名。
// compose.yaml の ${MC_LEVEL:-world} と合わせる。
const defaultLevel = "world"

// 保持ポリシーの既定値。.env のコメントと合わせる。
const (
	defaultKeepCount = 10
	defaultKeepDays  = 0
)

// 待ち時間の上限。COLD 取得の停止と起動に使う。
const (
	stopTimeout  = 90 * time.Second
	startTimeout = 300 * time.Second
)

var (
	// ErrInvalidMode は取得方式が未知であることを表す。
	ErrInvalidMode = errors.New("バックアップの取得方式が不正です")
	// ErrConfirmationRequired はバージョンや名前の警告が承諾されていないことを表す。
	ErrConfirmationRequired = errors.New("復元の内容が承諾されていません")
	// ErrUnknownArchiveLevel はアーカイブ内のワールド名を判定できないことを表す。
	ErrUnknownArchiveLevel = errors.New("アーカイブに含まれるワールドの名前を判定できません")
)

// restoreMargin は展開に必要な空き容量の余裕。
//
// 展開後のサイズちょうどで始めると、途中でディスクが埋まって
// 中断した状態が残る。復元の途中で止まるのが最悪なので余裕を取る。
const restoreMargin = 1.2

// Mode は取得方式。
type Mode int

const (
	// ModeHot は稼働させたまま取る。save-off / save-all / save-on を伴う。
	ModeHot Mode = iota + 1
	// ModeCold は停止してから取る。docker compose down / up を伴う。
	ModeCold
)

// Config は UseCase の依存。
type Config struct {
	Runtime    port.ContainerRuntime
	Console    port.ServerConsole
	Store      port.BackupStore
	Worlds     port.WorldRepository
	Config     port.ServerConfig
	Levels     port.LevelReader
	Operations *operations.Manager
	// Clock は現在時刻。未設定なら実時刻を使う。
	Clock port.Clock
}

// UseCase はバックアップの操作。
type UseCase struct {
	cfg Config
}

// New は UseCase を作る。
func New(cfg Config) (*UseCase, error) {
	if cfg.Runtime == nil || cfg.Console == nil || cfg.Store == nil ||
		cfg.Worlds == nil || cfg.Config == nil || cfg.Operations == nil {
		return nil, errors.New("runtime / console / store / worlds / config / operations が必要です")
	}
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}
	return &UseCase{cfg: cfg}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// settings は .env から読んだ、取得に必要な設定一式。
type settings struct {
	level   world.Name
	version string
	policy  backup.RetentionPolicy
}

// loadSettings は .env を読んで設定を組み立てる。
//
// 保持ポリシーの値が壊れていても取得そのものは止めない。既定値へ倒す。
// バックアップが取れないことの方が、世代数が想定と違うことより重い。
func (u *UseCase) loadSettings(ctx context.Context) (settings, error) {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return settings{}, err
	}
	return u.settingsFrom(snapshot)
}

func (u *UseCase) settingsFrom(snapshot port.ConfigSnapshot) (settings, error) {
	raw, ok := snapshot.Get(keyLevel)
	if !ok || raw == "" {
		raw = defaultLevel
	}
	level, err := world.NewName(raw)
	if err != nil {
		return settings{}, fmt.Errorf("%s の値が使えません: %w", keyLevel, err)
	}

	version, _ := snapshot.Get(keyVersion)
	return settings{level: level, version: version, policy: policyFrom(snapshot)}, nil
}

// policyFrom は .env から保持ポリシーを読む。
// 解釈できない値は既定値に倒す。ここで失敗させると、設定を直すまで
// バックアップが 1 件も取れなくなる。
func policyFrom(snapshot port.ConfigSnapshot) backup.RetentionPolicy {
	policy, err := backup.NewRetentionPolicy(
		intFrom(snapshot, keyKeep, defaultKeepCount),
		intFrom(snapshot, keyKeepDays, defaultKeepDays),
	)
	if err != nil {
		policy, _ = backup.NewRetentionPolicy(defaultKeepCount, defaultKeepDays)
	}
	return policy
}

func intFrom(snapshot port.ConfigSnapshot, key string, fallback int) int {
	raw, ok := snapshot.Get(key)
	if !ok || raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

// isRunning はコンテナが動いているかを返す。
// 状態を取れない場合は「動いていない」として扱い、RCON を呼ばない。
// 停止中に RCON を呼ぶと接続できずに失敗するだけになる。
func (u *UseCase) isRunning(ctx context.Context) bool {
	status, err := u.cfg.Runtime.Status(ctx)
	if err != nil {
		return false
	}
	return status.State == server.ContainerRunning
}
