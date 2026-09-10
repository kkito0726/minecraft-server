package worldfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// copyBufferSize はストリーミングに使うバッファの大きさ。
// Pi では MC が既に 2.7GB 使うため、内容を丸ごとメモリに載せない。
const copyBufferSize = 32 * 1024

// Copy はワールドを複製する。
//
// session.lock は含めない。起動時に作り直されるため持ち込む意味がない。
// 途中で失敗したら、作りかけのディレクトリを消す。中途半端なワールドが
// 一覧に出ると、利用者はそれが使えるものだと誤解する。
func (r *Repository) Copy(
	ctx context.Context,
	src, dst world.Name,
	progress port.Progress,
) (err error) {
	srcDir, err := r.requireExisting(ctx, src)
	if err != nil {
		return err
	}
	dstDir, err := r.requireAbsent(ctx, dst)
	if err != nil {
		return err
	}

	total, err := dirSize(srcDir)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = os.RemoveAll(dstDir)
		}
	}()

	buf := make([]byte, copyBufferSize)
	c := &copier{srcDir: srcDir, dstDir: dstDir, buf: buf, total: total, progress: progress}

	return filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, walkErr error) error {
		return c.visit(ctx, p, d, walkErr)
	})
}

// copier は複製の途中経過を持ち回る。
type copier struct {
	srcDir   string
	dstDir   string
	buf      []byte
	total    int64
	done     int64
	progress port.Progress
}

// visit は 1 エントリを複製する。
func (c *copier) visit(ctx context.Context, p string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return fmt.Errorf("走査できません (%s): %w", p, walkErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	rel, err := filepath.Rel(c.srcDir, p)
	if err != nil {
		return fmt.Errorf("相対パスを解決できません (%s): %w", p, err)
	}
	target := filepath.Join(c.dstDir, rel)

	if d.IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if c.skip(d) {
		return nil
	}

	n, err := copyFile(p, target, c.buf)
	if err != nil {
		return err
	}
	c.done += n
	if c.progress != nil {
		c.progress(c.done, c.total)
	}
	return nil
}

// skip は複製しないエントリかを返す。
//
// session.lock は起動時に作り直されるため持ち込む意味がない。
// シンボリックリンクは、リンク先を経由して data/ の外を参照させないため除く。
func (c *copier) skip(d fs.DirEntry) bool {
	return d.Name() == sessionLockName || d.Type()&fs.ModeSymlink != 0
}

func copyFile(src, dst string, buf []byte) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("読み込めません (%s): %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("ディレクトリを作れません (%s): %w", filepath.Dir(dst), err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("書き込めません (%s): %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	n, err := io.CopyBuffer(out, in, buf)
	if err != nil {
		return 0, fmt.Errorf("複製できません (%s): %w", src, err)
	}
	return n, nil
}
