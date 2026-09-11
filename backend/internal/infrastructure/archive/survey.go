package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// SurveyResult は持ち込まれたアーカイブの中身。
type SurveyResult struct {
	// LevelDatEntries は level.dat のエントリ名。ワールドの見分けに使う。
	LevelDatEntries []string
	// TotalBytes は展開後の合計サイズ。
	TotalBytes int64
	// EntryCount はファイルの数（ディレクトリを除く）。
	EntryCount int
}

/*
Survey は持ち込まれたアーカイブの中身を、展開せずに調べる。

Inspect と違って data/ 配下であることを要求しない。配布されている
ワールドは <名前>/level.dat の形をしており、要求してしまうと
「中身を見てどう取り込むか決める」こと自体ができなくなる。

緩めるのは置き場所の制限だけで、安全の制限ではない。展開先の外へ
出る経路（.. / 絶対パス / シンボリックリンク）はここでも塞ぐ。
*/
func Survey(ctx context.Context, src string) (SurveyResult, error) {
	if err := ctx.Err(); err != nil {
		return SurveyResult{}, err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return SurveyResult{}, fmt.Errorf("アーカイブを開けません: %w", err)
	}
	defer func() { _ = r.Close() }()

	var out SurveyResult
	for _, f := range r.File {
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return SurveyResult{}, err
		}
		cleaned, _, err := normalizeEntryName(f.Name)
		if err != nil {
			return SurveyResult{}, err
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}

		out.EntryCount++
		out.TotalBytes += int64(f.UncompressedSize64)
		if path.Base(cleaned) == "level.dat" {
			out.LevelDatEntries = append(out.LevelDatEntries, cleaned)
		}
	}
	return out, nil
}

/*
Rewrap はアーカイブの全エントリに prefix を前置した新しいアーカイブを作る。

外部のワールド（<名前>/...）を data/ 配下へ移すために使う。展開側の
allowedRoot を緩める代わりに入口で形を揃えるので、Extract の
不変条件は 1 文字も変わらない。

結果が展開先の外を指すなら、1 バイトも残さず拒否する。
*/
func Rewrap(ctx context.Context, src, dest, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("アーカイブを開けません: %w", err)
	}
	defer func() { _ = r.Close() }()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("包み直し先を作れません: %w", err)
	}
	// 途中で失敗したら消す。中途半端なアーカイブを残さない。
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(dest)
		}
	}()

	if err := copyEntries(ctx, r, out, prefix); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("包み直しを確定できません: %w", err)
	}
	committed = true
	return nil
}

func copyEntries(ctx context.Context, r *zip.ReadCloser, out io.Writer, prefix string) error {
	w := zip.NewWriter(out)
	registerFastCompressor(w)

	buf := make([]byte, copyBufferSize)
	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return err
		}
		cleaned, _, err := normalizeEntryName(f.Name)
		if err != nil {
			return err
		}
		// 前置した結果が data/ 配下に収まることを、書く前に確かめる。
		name, err := safeEntryName(prefix + cleaned)
		if err != nil {
			return err
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if err := copyOneEntry(w, f, name, buf); err != nil {
			return err
		}
	}
	return w.Close()
}

func copyOneEntry(w *zip.Writer, f *zip.File, name string, buf []byte) error {
	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("エントリを開けません (%s): %w", f.Name, err)
	}
	defer func() { _ = src.Close() }()

	dst, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("エントリを書けません (%s): %w", name, err)
	}
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		return fmt.Errorf("エントリを写せません (%s): %w", name, err)
	}
	return nil
}
