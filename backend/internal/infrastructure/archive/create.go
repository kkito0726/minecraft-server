package archive

import (
	"archive/zip"
	"compress/flate"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Create はバックアップのアーカイブを作る。

// Source はアーカイブに含める対象。
type Source struct {
	// Root は Path を解決する基準となるディレクトリ。
	Root string
	// Path は Root からの相対パス。ファイルでもディレクトリでもよい。
	// アーカイブ内のエントリ名もこれになる。
	Path string
	// ExcludeNames は除外するファイル名（ディレクトリ部分を含まない）。
	// session.lock は起動時に作り直されるため持ち込まない。
	ExcludeNames []string
}

func (s Source) excludes(name string) bool {
	for _, e := range s.ExcludeNames {
		if e == name {
			return true
		}
	}
	return false
}

// Create はアーカイブを作る。
//
// 一時ファイルへ書いてから名前を変更する。途中で失敗しても、
// 中途半端なアーカイブが一覧に現れることはない。
func Create(ctx context.Context, dest string, sources []Source, progress Progress) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".part-*")
	if err != nil {
		return fmt.Errorf("一時ファイルを作れません: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = writeArchive(ctx, tmp, sources, progress); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("アーカイブを同期できません: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("アーカイブを閉じられません: %w", err)
	}
	if err = os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("アーカイブを配置できません: %w", err)
	}
	return nil
}

// registerFastCompressor は圧縮を BestSpeed に固定する。
//
// .mca は既に zlib で圧縮済み。最大圧縮は Pi の CPU を無駄に使うだけで
// ほとんど縮まらない。アーカイブを作る経路すべてで同じ設定を使う。
func registerFastCompressor(zw *zip.Writer) {
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestSpeed)
	})
}

func writeArchive(ctx context.Context, w io.Writer, sources []Source, progress Progress) error {
	zw := zip.NewWriter(w)
	registerFastCompressor(zw)

	var done int64
	buf := make([]byte, copyBufferSize)

	for _, src := range sources {
		if err := addSource(ctx, zw, src, buf, &done, progress); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("アーカイブを閉じられません: %w", err)
	}
	return nil
}

func addSource(
	ctx context.Context,
	zw *zip.Writer,
	src Source,
	buf []byte,
	done *int64,
	progress Progress,
) error {
	base := filepath.Join(src.Root, src.Path)
	info, err := os.Stat(base)
	if err != nil {
		return fmt.Errorf("バックアップ対象が見つかりません (%s): %w", src.Path, err)
	}

	if !info.IsDir() {
		return addFile(ctx, zw, base, src.Path, buf, done, progress)
	}

	return filepath.Walk(base, func(p string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("走査できません (%s): %w", p, walkErr)
		}
		if src.excludes(fi.Name()) {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// 空のディレクトリもエントリとして残す。data/world/datapacks は
		// 既定で空だが、docs の zip -qr は保持するため往復で差分を出さない。
		if fi.IsDir() {
			return addEmptyDir(zw, src, p)
		}
		// シンボリックリンクはアーカイブに含めない。
		// 展開側でも拒否するが、作る側でも持ち込まない。
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(src.Root, p)
		if err != nil {
			return fmt.Errorf("相対パスを解決できません (%s): %w", p, err)
		}
		return addFile(ctx, zw, p, filepath.ToSlash(rel), buf, done, progress)
	})
}

// addEmptyDir は中身の無いディレクトリをエントリとして書く。
// 中身があるディレクトリは、ファイルの展開時に自動で作られるので飛ばす。
func addEmptyDir(zw *zip.Writer, src Source, dirPath string) error {
	empty, err := isEmptyDir(dirPath)
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}

	rel, err := filepath.Rel(src.Root, dirPath)
	if err != nil {
		return fmt.Errorf("相対パスを解決できません (%s): %w", dirPath, err)
	}
	if _, err := zw.Create(filepath.ToSlash(rel) + "/"); err != nil {
		return fmt.Errorf("ディレクトリのエントリを作れません (%s): %w", rel, err)
	}
	return nil
}

func isEmptyDir(dirPath string) (bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, fmt.Errorf("ディレクトリを読めません (%s): %w", dirPath, err)
	}
	return len(entries) == 0, nil
}

func addFile(
	ctx context.Context,
	zw *zip.Writer,
	srcPath, entryName string,
	buf []byte,
	done *int64,
	progress Progress,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("ファイルを開けません (%s): %w", srcPath, err)
	}
	defer func() { _ = f.Close() }()

	w, err := zw.Create(entryName)
	if err != nil {
		return fmt.Errorf("エントリを作れません (%s): %w", entryName, err)
	}

	n, err := io.CopyBuffer(w, f, buf)
	if err != nil {
		return fmt.Errorf("書き込めません (%s): %w", entryName, err)
	}

	*done += n
	progress.report(*done, 0)
	return nil
}
