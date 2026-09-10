package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extract はアーカイブを展開する。

// ExtractOptions は展開時の指定。
type ExtractOptions struct {
	// RewritePrefix はエントリ名の接頭辞を置き換える。
	//
	// 復元先のワールド名を変える場合に使う。これが無いと、
	// アーカイブ内の data/world/ がそのまま戻り、稼働中の
	// data/creative/ は変化しないまま「復元したのに何も起きない」ことになる。
	RewritePrefix map[string]string
}

func (o ExtractOptions) rewrite(name string) string {
	for from, to := range o.RewritePrefix {
		if after, ok := strings.CutPrefix(name, from); ok {
			return to + after
		}
	}
	return name
}

// Extract はアーカイブを展開する。
//
// 危険なエントリが 1 つでもあれば、**1 バイトも書かずに**アーカイブ全体を拒否する。
// 途中まで書いてから気づくと、中途半端なファイルが残って復旧が難しくなる。
func Extract(
	ctx context.Context,
	src, destRoot string,
	opts ExtractOptions,
	progress Progress,
) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("アーカイブを開けません (%s): %w", src, err)
	}
	defer func() { _ = r.Close() }()

	plan, total, err := buildPlan(r, opts)
	if err != nil {
		return err
	}

	buf := make([]byte, copyBufferSize)
	var done int64
	for _, item := range plan {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := extractOne(item, destRoot, buf)
		if err != nil {
			return err
		}
		done += n
		progress.report(done, total)
	}
	return nil
}

// planItem は展開する 1 エントリ。検証済みの名前を持つ。
type planItem struct {
	// file はディレクトリのエントリでは nil。
	file *zip.File
	// name は検証と書き換えを終えた、展開先からの相対パス。
	name  string
	isDir bool
}

// buildPlan は展開前に全エントリを検証する。
// 1 つでも安全でなければエラーを返し、呼び出し側は何も書かない。
func buildPlan(r *zip.ReadCloser, opts ExtractOptions) ([]planItem, int64, error) {
	plan := make([]planItem, 0, len(r.File))
	var total int64

	for _, f := range r.File {
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return nil, 0, err
		}
		// 書き換え前後の両方を検証する。書き換えの結果が
		// 展開先の外を指すこともあるため。
		if _, err := safeEntryName(f.Name); err != nil {
			return nil, 0, err
		}
		rewritten, err := safeEntryName(opts.rewrite(f.Name))
		if err != nil {
			return nil, 0, err
		}

		if strings.HasSuffix(f.Name, "/") {
			plan = append(plan, planItem{name: rewritten, isDir: true})
			continue
		}

		plan = append(plan, planItem{file: f, name: rewritten})
		total += int64(f.UncompressedSize64)
	}
	return plan, total, nil
}

func extractOne(item planItem, destRoot string, buf []byte) (int64, error) {
	dest := filepath.Join(destRoot, filepath.FromSlash(item.name))

	if item.isDir {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return 0, fmt.Errorf("ディレクトリを作れません (%s): %w", dest, err)
		}
		return 0, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("ディレクトリを作れません (%s): %w", filepath.Dir(dest), err)
	}

	rc, err := item.file.Open()
	if err != nil {
		return 0, fmt.Errorf("エントリを開けません (%s): %w", item.name, err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("ファイルを作れません (%s): %w", dest, err)
	}
	defer func() { _ = out.Close() }()

	n, err := io.CopyBuffer(out, rc, buf)
	if err != nil {
		return 0, fmt.Errorf("展開できません (%s): %w", item.name, err)
	}
	return n, nil
}
