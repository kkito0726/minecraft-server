package backupctl

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// RestoreTarget はどのワールド名として復元するか。
type RestoreTarget int

const (
	// TargetArchiveLevel はアーカイブのワールド名で復元する。既定。
	// MC_LEVEL もそちらへ切り替える。
	TargetArchiveLevel RestoreTarget = iota
	// TargetCurrentLevel は展開時にパスを書き換え、
	// 現在稼働中のワールド名として復元する。
	TargetCurrentLevel
)

// RestoreRequest は復元の要求。
type RestoreRequest struct {
	BackupID string
	Target   RestoreTarget
	// AcknowledgeVersionWarning は事前確認の警告を承諾したか。
	AcknowledgeVersionWarning bool
	// ConfirmLevelName は復元先のワールド名。完全一致していなければ拒否する。
	ConfirmLevelName string
}

// Preflight は復元前の確認結果。副作用を持たない。
type Preflight struct {
	Backup            Entry
	CurrentLevel      world.Name
	CurrentVersion    shared.WorldVersion
	ConfiguredVersion string
	Decision          backup.RestoreDecision
	RequiredBytes     int64
	AvailableBytes    int64
}

// restoreSteps は復元の手順。画面にそのまま出る。
var restoreSteps = []string{
	"復元の内容を確認しています",
	"空き容量を確認しています",
	"サーバーを停止しています",
	"現在のワールドを退避しています",
	"アーカイブを展開しています",
	"設定を更新しています",
	"サーバーを起動しています",
}

// PreflightRestore は復元前の確認を行う。副作用を一切持たない（REQ-008）。
//
// アーカイブ内の level.dat を、zip 全体を展開せずに読んでバージョンを判定する。
func (u *UseCase) PreflightRestore(ctx context.Context, backupID string) (Preflight, error) {
	id, err := backup.NewID(backupID)
	if err != nil {
		return Preflight{}, err
	}
	set, err := u.loadSettings(ctx)
	if err != nil {
		return Preflight{}, err
	}
	return u.preflight(ctx, id, set)
}

func (u *UseCase) preflight(ctx context.Context, id backup.ID, set settings) (Preflight, error) {
	info, err := u.cfg.Store.Inspect(ctx, id)
	if err != nil {
		return Preflight{}, err
	}

	archived := u.inspect(ctx, id)
	current := u.cfg.Levels.ReadWorld(ctx, set.level)
	declared := backup.ParseName(id.String())

	available, err := u.cfg.Worlds.AvailableBytes(ctx)
	if err != nil {
		return Preflight{}, err
	}

	return Preflight{
		Backup: Entry{
			ID:              id,
			DeclaredVersion: declared.Version,
			DeclaredLevel:   declared.Level,
			ArchiveLevel:    archived.level,
			EntryRoots:      archived.roots,
			Version:         archived.version,
		},
		CurrentLevel:      set.level,
		CurrentVersion:    current,
		ConfiguredVersion: set.version,
		Decision: backup.DecideRestore(
			archived.version, current, archived.level, set.level.String()),
		RequiredBytes:  int64(float64(info.TotalBytes) * restoreMargin),
		AvailableBytes: available,
	}, nil
}

// Restore はバックアップから復元する。
//
// 手順は「確認 → 停止 → 退避 → 展開 → 設定 → 起動」。
// **退避が展開より先**であることが最重要の不変条件になっている。
// 既存のディレクトリに上書き展開すると、アーカイブに含まれない新しい
// region ファイルが残り、古い地形と新しい地形が同居した壊れた
// ワールドになる。しかも起動はするので、気づくのは現地を訪れたときになる。
//
// 退避は mv であって rm ではない。展開に失敗しても元に戻せる。
func (u *UseCase) Restore(ctx context.Context, req RestoreRequest) (operations.Handle, error) {
	id, err := backup.NewID(req.BackupID)
	if err != nil {
		return operations.Handle{}, err
	}
	set, err := u.loadSettings(ctx)
	if err != nil {
		return operations.Handle{}, err
	}

	pre, err := u.preflight(ctx, id, set)
	if err != nil {
		return operations.Handle{}, err
	}
	target, err := u.resolveTarget(req.Target, pre, set)
	if err != nil {
		return operations.Handle{}, err
	}
	if err := checkConfirmation(req, pre, target); err != nil {
		return operations.Handle{}, err
	}

	return u.cfg.Operations.Start(ctx, operation.KindBackupRestore, restoreSteps,
		func(ctx context.Context, r operations.Reporter) error {
			return u.runRestore(ctx, r, req, id, target)
		})
}

// resolveTarget は復元先のワールド名を決める。
func (u *UseCase) resolveTarget(
	target RestoreTarget, pre Preflight, set settings,
) (world.Name, error) {
	if target == TargetCurrentLevel {
		return set.level, nil
	}
	if pre.Backup.ArchiveLevel == "" {
		return world.Name{}, ErrUnknownArchiveLevel
	}
	return world.NewName(pre.Backup.ArchiveLevel)
}

// checkConfirmation は利用者の承諾を確かめる。
//
// 事前確認はサーバー側でもう一度実行してから判定する。画面が
// 「確認しました」と主張しても、その画面が古い判定に基づいている
// 可能性があるため信用しない（REQ-113）。
func checkConfirmation(req RestoreRequest, pre Preflight, target world.Name) error {
	if req.ConfirmLevelName != target.String() {
		return fmt.Errorf("%w: %q と入力してください",
			world.ErrConfirmationMismatch, target)
	}
	if pre.Decision.RequiresConfirmation && !req.AcknowledgeVersionWarning {
		return fmt.Errorf("%w: %v", ErrConfirmationRequired, pre.Decision.Warnings)
	}
	return nil
}

func (u *UseCase) runRestore(
	ctx context.Context,
	r operations.Reporter,
	req RestoreRequest,
	id backup.ID,
	target world.Name,
) error {
	r.Attr("backup_id", id.String())
	r.Attr("world_name", target.String())

	// 1. 事前確認をもう一度実行する。ここまでの間に .env や
	//    ワールドが人の手で変わっている可能性がある。
	if err := r.Step(); err != nil {
		return err
	}
	set, err := u.loadSettings(ctx)
	if err != nil {
		return err
	}
	pre, err := u.preflight(ctx, id, set)
	if err != nil {
		return err
	}
	if err := checkConfirmation(req, pre, target); err != nil {
		return err
	}

	// 2. 空き容量。ここで止めれば、まだ何も壊していない。
	if err := r.Step(); err != nil {
		return err
	}
	if pre.AvailableBytes < pre.RequiredBytes {
		return fmt.Errorf("%w（必要 %d バイト、空き %d バイト）",
			port.ErrInsufficientSpace, pre.RequiredBytes, pre.AvailableBytes)
	}

	return u.applyRestore(ctx, r, req, id, target, set)
}

// applyRestore は実際にディスクを触る部分。
func (u *UseCase) applyRestore(
	ctx context.Context,
	r operations.Reporter,
	req RestoreRequest,
	id backup.ID,
	target world.Name,
	set settings,
) error {
	// 3. 停止。LEVEL は起動時に一度しか読まれないため、
	//    稼働したままでは設定の変更が反映されない。
	if err := r.Step(); err != nil {
		return err
	}
	wasRunning := u.isRunning(ctx)
	if wasRunning {
		if err := u.cfg.Runtime.Down(ctx, sinkTo(r)); err != nil {
			return err
		}
		if err := u.cfg.Runtime.WaitStopped(ctx, stopTimeout); err != nil {
			return err
		}
	} else {
		r.Logf(operation.LevelInfo, "サーバーは既に停止しています")
	}

	// 4. 退避。rm ではなく mv。展開に失敗しても戻せる。
	if err := r.Step(); err != nil {
		return err
	}
	quarantine, err := u.quarantine(ctx, r, target)
	if err != nil {
		return err
	}

	// 5. 展開。失敗したらここで巻き戻す。
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.extract(ctx, r, req, id, target); err != nil {
		u.rollback(ctx, r, target, quarantine)
		return err
	}

	// 6. 設定。復元先が現在の MC_LEVEL と違うときだけ書き換える。
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.switchLevel(ctx, r, target, set); err != nil {
		u.rollback(ctx, r, target, quarantine)
		return err
	}

	// 7. 起動。ここで失敗しても巻き戻さない。
	//    展開は終わっているので、戻すと復元した内容が失われる。
	if err := r.Step(); err != nil {
		return err
	}
	return u.startAfterRestore(ctx, r, wasRunning)
}

// quarantine は復元先のワールドを退避する。無ければ何もしない。
func (u *UseCase) quarantine(
	ctx context.Context, r operations.Reporter, target world.Name,
) (world.Quarantine, error) {
	exists, err := u.cfg.Worlds.Exists(ctx, target)
	if err != nil {
		return world.Quarantine{}, err
	}
	if !exists {
		r.Logf(operation.LevelInfo, "%s は存在しないため退避しません", target)
		return world.Quarantine{}, nil
	}

	q, err := u.cfg.Worlds.Quarantine(ctx, target, world.QuarantineFromRestore)
	if err != nil {
		return world.Quarantine{}, err
	}
	// 退避先は自動では消さない。動作を確認してから人が消す。
	r.Attr("quarantine_path", q.DirName())
	r.Logf(operation.LevelInfo,
		"%s を %s へ退避しました。動作を確認してから削除してください", target, q.DirName())
	return q, nil
}

func (u *UseCase) extract(
	ctx context.Context,
	r operations.Reporter,
	req RestoreRequest,
	id backup.ID,
	target world.Name,
) error {
	// アーカイブのワールド名で復元するなら書き換えは要らない。
	rewrite := ""
	if req.Target == TargetCurrentLevel {
		rewrite = target.String()
	}
	return u.cfg.Store.Extract(ctx, id, rewrite, func(done, total int64) {
		r.Bytes(done, total)
	})
}

// rollback は展開の途中までを消してから退避したものを戻す。
//
// 消さずに戻すと、展開されたファイルと元のファイルが混ざる。
//
// plugins / config / bukkit.yml / spigot.yml は退避していないため
// 戻らない。これらは unzip -o 相当の上書きであり、失われるのは
// バックアップ取得後に加えた変更だけなので、ワールドの地形が
// 壊れることに比べれば影響が小さいと判断している。
func (u *UseCase) rollback(
	ctx context.Context, r operations.Reporter, target world.Name, q world.Quarantine,
) {
	if err := u.cfg.Worlds.Remove(ctx, target); err != nil {
		r.Logf(operation.LevelError, "展開途中のワールドを削除できませんでした: %v", err)
	}
	if q.IsValid() {
		if err := u.cfg.Worlds.Restore(ctx, q, target); err != nil {
			r.Logf(operation.LevelError,
				"退避したワールドを戻せませんでした。%s に残っています: %v", q.DirName(), err)
		} else {
			r.Logf(operation.LevelWarn, "%s を元に戻しました", target)
		}
	}
	// 巻き戻したあとはサーバーを起こし直す。
	// 止めたまま放置すると、復元の失敗がサーバーの停止に化ける。
	if err := u.cfg.Runtime.Up(ctx, sinkTo(r)); err != nil {
		r.Logf(operation.LevelError, "サーバーの起動に失敗しました: %v", err)
		return
	}
	if err := u.cfg.Runtime.WaitReady(ctx, startTimeout); err != nil {
		r.Logf(operation.LevelError, "サーバーの起動完了を確認できませんでした: %v", err)
	}
}

func (u *UseCase) switchLevel(
	ctx context.Context, r operations.Reporter, target world.Name, set settings,
) error {
	if target == set.level {
		r.Logf(operation.LevelInfo, "稼働するワールドは %s のままです", target)
		return nil
	}

	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return err
	}
	if err := u.cfg.Config.Save(ctx, snapshot.With(keyLevel, target.String())); err != nil {
		return err
	}
	r.Logf(operation.LevelInfo, "稼働するワールドを %s に変更しました", target)
	return nil
}

// startAfterRestore は復元後にサーバーを起こす。
// もともと止まっていたなら起こさない。
func (u *UseCase) startAfterRestore(
	ctx context.Context, r operations.Reporter, wasRunning bool,
) error {
	if !wasRunning {
		r.Logf(operation.LevelInfo, "もともと停止していたため起動しません")
		return nil
	}
	if err := u.cfg.Runtime.Up(ctx, sinkTo(r)); err != nil {
		return err
	}
	return u.cfg.Runtime.WaitReady(ctx, startTimeout)
}
