// Package worldfs は data/ 配下のワールドディレクトリを操作する。
//
// 復元も削除も、元のディレクトリを消さずに退避する。
// docs/backup-restore.md が「rm ではなく mv で退避しておくと、
// 復元に失敗しても戻せる」と定めており、それをそのまま実装している。
package worldfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// ErrNotFound は対象のワールドが存在しないことを表す。
var ErrNotFound = errors.New("ワールドが見つかりません")

// ErrAlreadyExists は移動先や複製先が既に存在することを表す。
var ErrAlreadyExists = errors.New("同じ名前のワールドが既にあります")

// sessionLockName は起動時に作り直されるロックファイル。
// 複製に含めても害はないが、持ち込む意味もない。
const sessionLockName = "session.lock"

// levelDatName はワールドのメタデータ。
const levelDatName = "level.dat"

// VersionReader は level.dat からバージョンを読む。
type VersionReader interface {
	ReadFile(path string) (shared.WorldVersion, time.Time, error)
}

// Config は Repository の設定。
type Config struct {
	// DataDir はバインドマウントしている data/ のパス。
	DataDir string
	// Versions は level.dat の読み取り。nil なら常に「読めない」を返す。
	Versions VersionReader
}

// Repository はワールドディレクトリの操作。
type Repository struct {
	dataDir  string
	versions VersionReader
}

// New は Repository を作る。
func New(cfg Config) (*Repository, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("DataDir が指定されていません")
	}
	abs, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("DataDir の絶対パスを解決できません: %w", err)
	}
	return &Repository{dataDir: abs, versions: cfg.Versions}, nil
}

// pathOf はワールドのディレクトリパスを返す。
//
// world.Name が検証済みなので通常は安全だが、結果が DataDir の配下に
// 収まることを改めて確かめる。名前の検証と経路の検証の二重化。
func (r *Repository) pathOf(name string) (string, error) {
	p := filepath.Join(r.dataDir, name)
	rel, err := filepath.Rel(r.dataDir, p)
	if err != nil {
		return "", fmt.Errorf("パスを解決できません (%s): %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("data/ の外を指しています: %s", name)
	}
	return p, nil
}

// List はワールドの一覧を返す。
//
// level.dat を読めないワールドも含める。破損したものが 1 つあっても
// 一覧そのものが使えなくなってはいけない。
func (r *Repository) List(ctx context.Context, activeLevel world.Name) ([]world.World, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(r.dataDir)
	if err != nil {
		return nil, fmt.Errorf("data/ を読めません (%s): %w", r.dataDir, err)
	}

	var out []world.World
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// 予約名・退避名はワールドとして扱わない。world.NewName が弾く。
		n, err := world.NewName(e.Name())
		if err != nil {
			continue
		}
		w, err := r.describe(n, n == activeLevel)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

func (r *Repository) describe(n world.Name, active bool) (world.World, error) {
	dir, err := r.pathOf(n.String())
	if err != nil {
		return world.World{}, err
	}

	size, err := dirSize(dir)
	if err != nil {
		return world.World{}, err
	}

	version, lastPlayed := r.readVersion(filepath.Join(dir, levelDatName))
	_, lockErr := os.Stat(filepath.Join(dir, sessionLockName))

	return world.NewWorld(n, active, size, lastPlayed, version, lockErr == nil), nil
}

// readVersion は level.dat を読む。読めなければ「読めない」を返す。
func (r *Repository) readVersion(path string) (shared.WorldVersion, time.Time) {
	if r.versions == nil {
		return shared.UnreadableWorldVersion(), time.Time{}
	}
	v, lastPlayed, err := r.versions.ReadFile(path)
	if err != nil {
		return shared.UnreadableWorldVersion(), time.Time{}
	}
	return v, lastPlayed
}

// Exists はワールドが存在するかを返す。
func (r *Repository) Exists(ctx context.Context, name world.Name) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	dir, err := r.pathOf(name.String())
	if err != nil {
		return false, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("ワールドを確認できません (%s): %w", name, err)
	}
	return info.IsDir(), nil
}

// Remove はワールドを削除する。
func (r *Repository) Remove(ctx context.Context, name world.Name) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir, err := r.requireExisting(ctx, name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("ワールドを削除できません (%s): %w", name, err)
	}
	return nil
}

// Rename はワールドの名前を変える。
func (r *Repository) Rename(ctx context.Context, from, to world.Name) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	src, err := r.requireExisting(ctx, from)
	if err != nil {
		return err
	}
	dst, err := r.requireAbsent(ctx, to)
	if err != nil {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("ワールドを改名できません (%s -> %s): %w", from, to, err)
	}
	return nil
}

// requireExisting は存在を確かめてパスを返す。
func (r *Repository) requireExisting(ctx context.Context, name world.Name) (string, error) {
	ok, err := r.Exists(ctx, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return r.pathOf(name.String())
}

// requireAbsent は不在を確かめてパスを返す。
func (r *Repository) requireAbsent(ctx context.Context, name world.Name) (string, error) {
	ok, err := r.Exists(ctx, name)
	if err != nil {
		return "", err
	}
	if ok {
		return "", fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}
	return r.pathOf(name.String())
}

// AvailableBytes は data/ のあるファイルシステムの空き容量を返す。
func (r *Repository) AvailableBytes(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return availableBytes(r.dataDir)
}

// dirSize はディレクトリの合計サイズを返す。
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			// 走査中に消えたファイル。合計に含めないだけでよい。
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("サイズを計算できません (%s): %w", dir, err)
	}
	return total, nil
}

var _ = port.Progress(nil)
