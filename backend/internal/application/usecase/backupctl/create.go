package backupctl

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// ステップ名。画面にそのまま出る。
//
// ステップ数は固定にしてある。サーバーが停止している場合でも
// 「停止しています」を飛ばさず、何もせずに通過したことをログに残す。
// 進捗の分母が状況によって変わると、画面が段数を数え直すことになる。
var (
	hotSteps = []string{
		"ワールドの保存を止めています",
		"アーカイブを作成しています",
		"世代管理を適用しています",
	}
	coldSteps = []string{
		"サーバーを停止しています",
		"アーカイブを作成しています",
		"サーバーを起動しています",
		"世代管理を適用しています",
	}
)

// Create はバックアップを取得する。
//
// HOT は稼働させたまま取る。COLD は停止してから取る。
// 既定は HOT で、docs/backup-restore.md の検証済み手順と同じ。
func (u *UseCase) Create(ctx context.Context, mode Mode, note string) (operations.Handle, error) {
	steps, err := stepsFor(mode)
	if err != nil {
		return operations.Handle{}, err
	}

	return u.cfg.Operations.Start(ctx, operation.KindBackupCreate, steps,
		func(ctx context.Context, r operations.Reporter) error {
			return u.runCreate(ctx, r, mode, note)
		})
}

func stepsFor(mode Mode) ([]string, error) {
	switch mode {
	case ModeHot:
		return hotSteps, nil
	case ModeCold:
		return coldSteps, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidMode, mode)
	}
}

func (u *UseCase) runCreate(
	ctx context.Context, r operations.Reporter, mode Mode, note string,
) error {
	set, err := u.loadSettings(ctx)
	if err != nil {
		return err
	}

	name := backup.BuildName(set.version, set.level.String(), note, u.cfg.Clock.Now())
	id, err := backup.NewID(name)
	if err != nil {
		return err
	}
	r.Attr("backup_id", id.String())
	r.Attr("world_name", set.level.String())
	// ファイル名に使えるのは英数字と _ だけ。日本語のメモは丸ごと落ちる。
	// 黙って捨てると、利用者は付けたはずのメモが無いことに後から気づく。
	if note != "" && backup.NoteSlug(note) == "" {
		r.Logf(operation.LevelWarn,
			"メモ %q はファイル名に使える文字を含まないため付けませんでした", note)
	}

	stored, err := u.archive(ctx, r, mode, id, set.level)
	if err != nil {
		return err
	}
	r.Logf(operation.LevelInfo, "%s を作成しました（%d バイト）", id, stored.SizeBytes)

	// 世代管理は取得が成功したときにだけ適用する。
	// 失敗したときに適用すると、新しいものが増えていないのに古いものだけ消える。
	if err := r.Step(); err != nil {
		return err
	}
	return u.pruneAfterCreate(ctx, r, set)
}

func (u *UseCase) archive(
	ctx context.Context,
	r operations.Reporter,
	mode Mode,
	id backup.ID,
	level world.Name,
) (port.StoredBackup, error) {
	if mode == ModeCold {
		return u.archiveCold(ctx, r, id, level)
	}
	return u.archiveHot(ctx, r, id, level)
}

// archiveHot は稼働させたまま取る。
//
// save-off を送る前にロックへ記録を立て、save-on を送った後に降ろす。
// 逆順にすると、その隙間でプロセスが落ちたときに「保存は正常」と
// 誤って記録され、復旧の手がかりが消える。
func (u *UseCase) archiveHot(
	ctx context.Context,
	r operations.Reporter,
	id backup.ID,
	level world.Name,
) (port.StoredBackup, error) {
	if err := r.Step(); err != nil {
		return port.StoredBackup{}, err
	}

	// 停止中は保存も走っていないため save-off は不要で、
	// 呼んでも接続できずに失敗するだけになる。
	if u.isRunning(ctx) {
		r.MarkSaveDisabled(true)
		// save-on はアーカイブの作成が失敗しても必ず送る。
		// 忘れると以降の変更がディスクに書かれないのに症状が何も出ない。
		defer u.resumeSaving(ctx, r)

		if err := u.cfg.Console.SaveOff(ctx); err != nil {
			return port.StoredBackup{}, err
		}
		// save-all を省くと、メモリ上にしかないチャンクが取り込まれない。
		if err := u.cfg.Console.SaveAll(ctx); err != nil {
			return port.StoredBackup{}, err
		}
	} else {
		r.Logf(operation.LevelInfo, "サーバーが停止しているため保存の制御を省略します")
	}

	if err := r.Step(); err != nil {
		return port.StoredBackup{}, err
	}
	return u.write(ctx, r, id, level)
}

// resumeSaving は保存を再開し、ロックの記録を降ろす。
// アーカイブの作成が失敗しても必ず通る経路。
func (u *UseCase) resumeSaving(ctx context.Context, r operations.Reporter) {
	if err := u.cfg.Console.SaveOn(ctx); err != nil {
		r.Logf(operation.LevelError, "保存の再開に失敗しました: %v", err)
	}
	r.MarkSaveDisabled(false)
}

// archiveCold は停止してから取る。
//
// Down の完了を待たずに取ると、stop_grace_period（60 秒）の間に
// Paper が書いている region ファイルを掴む。COLD で取る意味が消える。
func (u *UseCase) archiveCold(
	ctx context.Context,
	r operations.Reporter,
	id backup.ID,
	level world.Name,
) (port.StoredBackup, error) {
	if err := r.Step(); err != nil {
		return port.StoredBackup{}, err
	}

	// 利用者が意図して止めているサーバーを、バックアップが勝手に起こしてはならない。
	wasRunning := u.isRunning(ctx)
	if wasRunning {
		if err := u.cfg.Runtime.Down(ctx, sinkTo(r)); err != nil {
			return port.StoredBackup{}, err
		}
	} else {
		r.Logf(operation.LevelInfo, "サーバーは既に停止しています")
	}

	// 取得に失敗してもサーバーは起こし直す。
	// 止めたまま放置すると、バックアップの失敗がサーバーの停止に化ける。
	defer u.resumeServer(ctx, r, wasRunning)

	if wasRunning {
		if err := u.cfg.Runtime.WaitStopped(ctx, stopTimeout); err != nil {
			return port.StoredBackup{}, err
		}
	}

	if err := r.Step(); err != nil {
		return port.StoredBackup{}, err
	}
	return u.write(ctx, r, id, level)
}

// resumeServer は停止させたサーバーを起こし直す。
// もともと止まっていた場合はステップだけ進めて何もしない。
func (u *UseCase) resumeServer(ctx context.Context, r operations.Reporter, wasRunning bool) {
	if err := r.Step(); err != nil {
		return
	}
	if !wasRunning {
		r.Logf(operation.LevelInfo, "もともと停止していたため起動しません")
		return
	}
	if err := u.cfg.Runtime.Up(ctx, sinkTo(r)); err != nil {
		r.Logf(operation.LevelError, "サーバーの起動に失敗しました: %v", err)
		return
	}
	if err := u.cfg.Runtime.WaitReady(ctx, startTimeout); err != nil {
		r.Logf(operation.LevelError, "サーバーの起動完了を確認できませんでした: %v", err)
	}
}

func (u *UseCase) write(
	ctx context.Context, r operations.Reporter, id backup.ID, level world.Name,
) (port.StoredBackup, error) {
	return u.cfg.Store.Create(ctx, id, level, func(done, total int64) {
		r.Bytes(done, total)
	})
}

// pruneAfterCreate は取得の直後に保持ポリシーを適用する。
//
// 削除に失敗しても取得そのものは成功として扱う。アーカイブは既に
// できており、古いものが残ることは損失ではない。
func (u *UseCase) pruneAfterCreate(ctx context.Context, r operations.Reporter, set settings) error {
	result, err := u.prune(ctx, set.policy, false)
	if err != nil {
		r.Logf(operation.LevelWarn, "世代管理の適用に失敗しました: %v", err)
		return nil
	}
	for _, id := range result.DeletedIDs {
		r.Logf(operation.LevelInfo, "古いバックアップを削除しました: %s", id)
	}
	return nil
}

func sinkTo(r operations.Reporter) port.LogSink {
	return func(line string) { r.Logf(operation.LevelInfo, "%s", line) }
}
