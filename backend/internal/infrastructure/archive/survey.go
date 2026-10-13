package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
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

Inspect と違って形を判定しない。取り込みの段階では、ワールドが
あるかどうか・いくつあるかを見てから受け取るかを決めるため、
形の確定より先に中身を知る必要がある。

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
		if isDirEntry(f.Name) {
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
