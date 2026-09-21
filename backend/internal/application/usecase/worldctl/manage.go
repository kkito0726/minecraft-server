package worldctl

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/settings"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// copyHeadroom は複製に必要とする空き容量の余裕。
// 途中で容量が尽きると中途半端なディレクトリが残る。
const copyHeadroom = 1.1

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

	listed, err := u.cfg.Worlds.List(ctx, active)
	if err != nil {
		return Listing{}, err
	}
	// 切り替える前にハードコアだと分かるように印を足す。level.dat は
	// バージョンのためにも読んでいるので 2 度読みになるが、数 KB の圧縮
	// ファイルで、ワールドは数個しか無い。読む層を分けておく方を取る。
	worlds := make([]world.World, len(listed))
	for i, w := range listed {
		worlds[i] = w.WithHardcore(u.cfg.Levels.ReadSettings(ctx, w.Name()).Hardcore)
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
// CreateOptions は新しいワールドを作るときの指定。
//
// Mode と Difficulty が空なら .env の現在の値を使う。Hardcore だけは
// 空を表せないので常に明示として扱い、偽なら FALSE を書く。前に作った
// ハードコアのワールドの設定を、次のワールドが黙って引き継がないため。
//
// **いずれもサーバー全体の設定で、ワールドごとには持たない。** それでも
// 作成時に受け取るのは、ゲームモードとハードコアが効くのが生成の瞬間だから。
// 生成後に変えても、モードは新しく接続した人にしか効かず、ハードコアは
// level.dat に焼かれた値と食い違う。
type CreateOptions struct {
	Name       string
	Seed       string
	Mode       settings.GameMode
	Difficulty settings.Difficulty
	Hardcore   bool
	// Version は生成に使う版（MC_VERSION）。空なら現在の版のまま。
	Version string
}

func (u *UseCase) Create(ctx context.Context, opts CreateOptions) (operations.Handle, error) {
	target, err := world.NewName(opts.Name)
	if err != nil {
		return operations.Handle{}, err
	}
	if err := u.validateCreateOptions(opts); err != nil {
		return operations.Handle{}, err
	}
	if err := u.validateCreateVersion(ctx, opts.Version); err != nil {
		return operations.Handle{}, err
	}
	if err := u.requireAbsent(ctx, target); err != nil {
		return operations.Handle{}, err
	}

	return u.cfg.Operations.Start(ctx, operation.KindWorldCreate, createSteps(),
		func(ctx context.Context, r operations.Reporter) error {
			return u.runCreate(ctx, r, target, opts)
		})
}

// validateCreateOptions は生成前に値を確かめる。
//
// 書き込みを始めてから弾くと、サーバーを止めた後で失敗することになる。
func (u *UseCase) validateCreateOptions(opts CreateOptions) error {
	if opts.Mode != "" {
		if _, err := settings.ParseGameMode(string(opts.Mode)); err != nil {
			return err
		}
	}
	if opts.Difficulty != "" {
		if _, err := settings.ParseDifficulty(string(opts.Difficulty)); err != nil {
			return err
		}
	}
	return nil
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
	opts CreateOptions,
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
	if err := u.writeCreateSettings(ctx, target, opts); err != nil {
		return err
	}
	r.Attr("world_name", target.String())
	if opts.Seed != "" {
		r.Logf(operation.LevelInfo, "シード %s で生成します", opts.Seed)
	}
	if opts.Mode != "" {
		r.Logf(operation.LevelInfo, "ゲームモード %s で生成します", opts.Mode)
	}
	if opts.Version != "" {
		r.Logf(operation.LevelInfo, "版 %s で生成します。遊ぶ人はクライアントを %s にしてください",
			opts.Version, opts.Version)
	}
	if opts.Hardcore {
		// 後から外せない設定なので、ログにも必ず残す。
		r.Logf(operation.LevelWarn, "ハードコアで生成します（後から外せません）")
	}

	// 生成は既存ワールドを開くより時間がかかる。
	return u.startAndWait(ctx, r, false)
}

// writeCreateSettings は生成に効く .env のキーをまとめて書く。
//
// 1 回の書き込みにするのは、途中で失敗したときに「モードだけ変わって
// ワールドは作られていない」という半端な状態を残さないため。
func (u *UseCase) writeCreateSettings(
	ctx context.Context,
	name world.Name,
	opts CreateOptions,
) error {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return err
	}

	updated := snapshot.
		With(keyLevel, name.String()).
		With(keySeed, opts.Seed).
		With(keyHardcore, boolValue(opts.Hardcore))
	if opts.Mode != "" {
		updated = updated.With(keyMode, string(opts.Mode))
	}
	if opts.Version != "" {
		updated = updated.With(keyVersion, opts.Version)
	}

	// ハードコアでは Minecraft が難易度をハードに固定する。.env に別の値を
	// 残すと、画面の表示と実際の挙動が食い違う。指定を無視して hard を書く。
	if opts.Hardcore {
		updated = updated.With(keyDifficulty, string(settings.DifficultyHard))
	} else if opts.Difficulty != "" {
		updated = updated.With(keyDifficulty, string(opts.Difficulty))
	}
	return u.cfg.Config.Save(ctx, updated)
}

// boolValue は compose が受け取る形に揃える。
func boolValue(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
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
			"%w（必要 %d バイト、空き %d バイト）", port.ErrInsufficientSpace, need, available)
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
	if err := u.setLevel(ctx, r, target); err != nil {
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
