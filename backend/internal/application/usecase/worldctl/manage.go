package worldctl

import (
	"context"
	"errors"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// copyHeadroom は複製に必要とする空き容量の余裕。
// 途中で容量が尽きると中途半端なディレクトリが残る。
const copyHeadroom = 1.1

// ErrInsufficientSpace は空き容量が足りないことを表す。
var ErrInsufficientSpace = errors.New("空き容量が足りません")

// Listing はワールドと退避の一覧。
type Listing struct {
	Worlds      []world.World
	Quarantines []world.Quarantine
	ActiveLevel world.Name
}

// List はワールドと退避の一覧を返す。
func (u *UseCase) List(ctx context.Context) (Listing, error) {
	active, err := u.currentLevel(ctx)
	if err != nil {
		// .env に不正な名前が手で書かれていても一覧は返す。
		// 画面が全く開けなくなるより、稼働中の印が付かない方がまし。
		active = world.Name{}
	}

	worlds, err := u.cfg.Worlds.List(ctx, active)
	if err != nil {
		return Listing{}, err
	}
	quarantines, err := u.cfg.Worlds.ListQuarantines(ctx)
	if err != nil {
		return Listing{}, err
	}
	return Listing{Worlds: worlds, Quarantines: quarantines, ActiveLevel: active}, nil
}

// Create は新しいワールドを作って切り替える。
//
// ディレクトリは作らない。MC_LEVEL を新しい名前にして起動すれば
// Paper が生成する。シードを指定した場合は .env に書いてから起動し、
// 生成後は残さない。
func (u *UseCase) Create(ctx context.Context, name, seed string) (operations.Handle, error) {
	target, err := world.NewName(name)
	if err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireAbsent(ctx, target); err != nil {
		return operations.Handle{}, err
	}

	return u.cfg.Operations.Start(ctx, operation.KindWorldCreate, createSteps(),
		func(ctx context.Context, r operations.Reporter) error {
			return u.runCreate(ctx, r, target, seed)
		})
}

func createSteps() []string {
	return []string{
		"ワールドを保存しています",
		"サーバーを停止しています",
		"設定を書き換えています",
		"サーバーを起動しています",
		"ワールドの生成を待っています",
	}
}

func (u *UseCase) runCreate(
	ctx context.Context,
	r operations.Reporter,
	target world.Name,
	seed string,
) error {
	if err := r.Step(); err != nil {
		return err
	}
	u.saveIfRunning(ctx, r)

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.stop(ctx, r); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.setLevelWithSeed(ctx, target, seed); err != nil {
		return err
	}
	r.Attr("world_name", target.String())
	if seed != "" {
		r.Logf(operation.LevelInfo, "シード %s で生成します", seed)
	}

	// 生成は既存ワールドを開くより時間がかかる。
	return u.startAndWait(ctx, r, false)
}

// setLevelWithSeed は MC_LEVEL と MC_SEED を同時に書く。
func (u *UseCase) setLevelWithSeed(ctx context.Context, name world.Name, seed string) error {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return err
	}
	updated := snapshot.With(keyLevel, name.String()).With(keySeed, seed)
	return u.cfg.Config.Save(ctx, updated)
}

// Clone はワールドを複製する。切り替えはしない。
//
// 稼働中のワールドを複製する場合は、書き込み途中の region ファイルを
// 掴まないよう保存を止めてから複製する。save-on は必ず戻す。
func (u *UseCase) Clone(ctx context.Context, src, dst string) (operations.Handle, error) {
	source, err := world.NewName(src)
	if err != nil {
		return operations.Handle{}, err
	}
	destination, err := world.NewName(dst)
	if err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireExisting(ctx, source); err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireAbsent(ctx, destination); err != nil {
		return operations.Handle{}, err
	}

	steps := []string{"空き容量を確認しています", "ワールドを複製しています"}
	return u.cfg.Operations.Start(ctx, operation.KindWorldClone, steps,
		func(ctx context.Context, r operations.Reporter) error {
			return u.runClone(ctx, r, source, destination)
		})
}

func (u *UseCase) runClone(
	ctx context.Context,
	r operations.Reporter,
	source, destination world.Name,
) error {
	if err := r.Step(); err != nil {
		return err
	}
	if err := u.checkSpaceFor(ctx, source); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	r.Attr("world_name", destination.String())

	// 稼働中なら保存を止めてから複製する。save-on は defer で必ず戻す。
	// 忘れると以降の変更がディスクに書かれないのに症状が出ない。
	//
	// 記録は save-off の前に立て、save-on の後に降ろす。逆順にすると、
	// その隙間でプロセスが落ちたときに「保存は正常」と誤って記録され、
	// 次の起動での復旧の手がかりが消える。
	if u.shouldPauseSaving(ctx) {
		r.MarkSaveDisabled(true)
		defer func() {
			if err := u.cfg.Console.SaveOn(ctx); err != nil {
				r.Logf(operation.LevelError, "保存の再開に失敗しました: %v", err)
			}
			r.MarkSaveDisabled(false)
		}()
		if err := u.cfg.Console.SaveOff(ctx); err != nil {
			return err
		}
		if err := u.cfg.Console.SaveAll(ctx); err != nil {
			return err
		}
	}

	return u.cfg.Worlds.Copy(ctx, source, destination, func(done, total int64) {
		r.Bytes(done, total)
	})
}

// shouldPauseSaving は保存を止める必要があるかを返す。
func (u *UseCase) shouldPauseSaving(ctx context.Context) bool {
	if u.cfg.Console == nil {
		return false
	}
	status, err := u.cfg.Runtime.Status(ctx)
	return err == nil && status.State.IsUp()
}

// checkSpaceFor は複製に必要な空き容量があるかを確かめる。
func (u *UseCase) checkSpaceFor(ctx context.Context, source world.Name) error {
	worlds, err := u.cfg.Worlds.List(ctx, world.Name{})
	if err != nil {
		return err
	}

	var need int64
	for _, w := range worlds {
		if w.Name() == source {
			need = int64(float64(w.SizeBytes()) * copyHeadroom)
			break
		}
	}

	available, err := u.cfg.Worlds.AvailableBytes(ctx)
	if err != nil {
		return err
	}
	if available < need {
		return fmt.Errorf(
			"%w（必要 %d バイト、空き %d バイト）", ErrInsufficientSpace, need, available)
	}
	return nil
}

// Rename はワールドの名前を変える。
//
// 稼働中のワールドなら、停止してから改名し、MC_LEVEL も追随させて
// 起動し直す。停止せずに改名すると、サーバーが開いているディレクトリが
// 消えたことになる。
func (u *UseCase) Rename(ctx context.Context, from, to string) (operations.Handle, error) {
	source, err := world.NewName(from)
	if err != nil {
		return operations.Handle{}, err
	}
	target, err := world.NewName(to)
	if err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireExisting(ctx, source); err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireAbsent(ctx, target); err != nil {
		return operations.Handle{}, err
	}

	current, err := u.currentLevel(ctx)
	if err != nil {
		return operations.Handle{}, err
	}
	active := current == source

	return u.cfg.Operations.Start(ctx, operation.KindWorldRename, renameSteps(active),
		func(ctx context.Context, r operations.Reporter) error {
			return u.runRename(ctx, r, source, target, active)
		})
}

func renameSteps(active bool) []string {
	if !active {
		return []string{"ワールドを改名しています"}
	}
	return []string{
		"ワールドを保存しています",
		"サーバーを停止しています",
		"ワールドを改名しています",
		"設定を書き換えています",
		"サーバーを起動しています",
		"起動を確認しています",
	}
}

func (u *UseCase) runRename(
	ctx context.Context,
	r operations.Reporter,
	source, target world.Name,
	active bool,
) error {
	r.Attr("world_name", target.String())

	if !active {
		if err := r.Step(); err != nil {
			return err
		}
		return u.cfg.Worlds.Rename(ctx, source, target)
	}

	if err := r.Step(); err != nil {
		return err
	}
	u.saveIfRunning(ctx, r)

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.stop(ctx, r); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.cfg.Worlds.Rename(ctx, source, target); err != nil {
		return err
	}

	if err := r.Step(); err != nil {
		return err
	}
	if err := u.setLevel(ctx, target); err != nil {
		return err
	}

	return u.startAndWait(ctx, r, true)
}

// Delete はワールドを削除する。
//
// 既定では即座に消さず退避する。稼働中のワールドは削除できない。
// 消すと復旧できないため、先に切り替えてもらう。
func (u *UseCase) Delete(
	ctx context.Context,
	name, confirmName string,
	quarantine bool,
) (operations.Handle, error) {
	target, err := world.NewName(name)
	if err != nil {
		return operations.Handle{}, err
	}
	if confirmName != name {
		return operations.Handle{}, fmt.Errorf(
			"%w（入力 %q、対象 %q）", world.ErrConfirmationMismatch, confirmName, name)
	}
	if err := u.requireExisting(ctx, target); err != nil {
		return operations.Handle{}, err
	}

	current, err := u.currentLevel(ctx)
	if err != nil {
		return operations.Handle{}, err
	}
	if current == target {
		return operations.Handle{}, world.ErrActiveWorld
	}

	step := "ワールドを削除しています"
	if quarantine {
		step = "ワールドを退避しています"
	}
	return u.cfg.Operations.Start(ctx, operation.KindWorldDelete, []string{step},
		func(ctx context.Context, r operations.Reporter) error {
			return u.runDelete(ctx, r, target, quarantine)
		})
}

func (u *UseCase) runDelete(
	ctx context.Context,
	r operations.Reporter,
	target world.Name,
	quarantine bool,
) error {
	if err := r.Step(); err != nil {
		return err
	}
	r.Attr("world_name", target.String())

	if !quarantine {
		return u.cfg.Worlds.Remove(ctx, target)
	}

	q, err := u.cfg.Worlds.Quarantine(ctx, target, world.QuarantineFromDelete)
	if err != nil {
		return err
	}
	r.Attr("quarantine_path", q.DirName())
	r.Logf(operation.LevelInfo,
		"%s へ退避しました。完全に削除するには一覧から実行してください", q.DirName())
	return nil
}

// PurgeQuarantine は退避されたディレクトリを完全に削除する。
//
// システムは自動削除しない。docs の「問題なく動くことを確認してから
// 削除する」という手順を、画面からの明示的な操作に落としている。
func (u *UseCase) PurgeQuarantine(ctx context.Context, dirName string) (int64, error) {
	list, err := u.cfg.Worlds.ListQuarantines(ctx)
	if err != nil {
		return 0, err
	}
	for _, q := range list {
		if q.DirName() == dirName {
			return u.cfg.Worlds.RemoveQuarantine(ctx, q)
		}
	}
	return 0, fmt.Errorf("%w: 退避 %s", world.ErrNotFound, dirName)
}

func (u *UseCase) requireExisting(ctx context.Context, name world.Name) error {
	ok, err := u.cfg.Worlds.Exists(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: ワールド %s", world.ErrNotFound, name)
	}
	return nil
}

func (u *UseCase) requireAbsent(ctx context.Context, name world.Name) error {
	ok, err := u.cfg.Worlds.Exists(ctx, name)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("%w: %s", world.ErrAlreadyExists, name)
	}
	return nil
}
