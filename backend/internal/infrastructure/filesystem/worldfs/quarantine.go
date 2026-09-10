package worldfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// Quarantine はワールドを退避先へ移動する。削除はしない。
//
// docs/backup-restore.md が「rm ではなく mv で退避しておくと、
// 復元に失敗しても戻せる」と定めている。同じファイルシステム内の
// 移動なので、30MB のワールドでも一瞬で終わる。
func (r *Repository) Quarantine(
	ctx context.Context,
	name world.Name,
	kind world.QuarantineKind,
) (world.Quarantine, error) {
	src, err := r.requireExisting(ctx, name)
	if err != nil {
		return world.Quarantine{}, err
	}

	dirName := world.NewQuarantineName(name, kind, time.Now())
	dst := filepath.Join(r.dataDir, dirName)

	if _, err := os.Stat(dst); err == nil {
		return world.Quarantine{}, fmt.Errorf("退避先が既にあります: %s", dirName)
	}

	size, err := dirSize(src)
	if err != nil {
		return world.Quarantine{}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return world.Quarantine{}, fmt.Errorf("退避できません (%s): %w", name, err)
	}

	return world.ParseQuarantine(dirName, size)
}

// Restore は退避したディレクトリを元の位置へ戻す。
//
// 復元の展開が失敗したときのロールバックに使う。
// 展開途中のディレクトリがあれば先に消す。
func (r *Repository) Restore(ctx context.Context, q world.Quarantine, to world.Name) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	src := filepath.Join(r.dataDir, q.DirName())
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("退避先が見つかりません (%s): %w", q.DirName(), err)
	}

	dst, err := r.pathOf(to.String())
	if err != nil {
		return err
	}
	// 展開途中のものを片付ける。残したまま rename すると失敗する。
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("展開途中のディレクトリを削除できません (%s): %w", to, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("退避から戻せません (%s -> %s): %w", q.DirName(), to, err)
	}
	return nil
}

// ListQuarantines は退避されたディレクトリの一覧を返す。
//
// システムはこれを自動削除しない。docs の「問題なく動くことを確認してから
// 削除する」という手順を、画面からの明示的な操作に落としている。
func (r *Repository) ListQuarantines(ctx context.Context) ([]world.Quarantine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(r.dataDir)
	if err != nil {
		return nil, fmt.Errorf("data/ を読めません (%s): %w", r.dataDir, err)
	}

	var out []world.Quarantine
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		size, sizeErr := dirSize(filepath.Join(r.dataDir, e.Name()))
		if sizeErr != nil {
			continue
		}
		q, parseErr := world.ParseQuarantine(e.Name(), size)
		if parseErr != nil {
			continue
		}
		out = append(out, q)
	}
	return out, nil
}

// RemoveQuarantine は退避されたディレクトリを完全に削除する。
func (r *Repository) RemoveQuarantine(ctx context.Context, q world.Quarantine) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	dir := filepath.Join(r.dataDir, q.DirName())
	size, err := dirSize(dir)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, fmt.Errorf("退避を削除できません (%s): %w", q.DirName(), err)
	}
	return size, nil
}
