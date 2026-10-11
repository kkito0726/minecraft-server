package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrRemapIncomplete は包み直した結果に必要なエントリが無いことを表す。
var ErrRemapIncomplete = errors.New("包み直した結果が不完全です")

/*
RemapOptions は包み直しの指定。

Rewrap が接頭辞を前置するだけなのに対して、こちらはエントリごとに
名前を決める。置き場所の規則そのものはドメインが持ち、この層は
zip を読み書きする手順だけを受け持つ。
*/
type RemapOptions struct {
	// Rename はエントリ名を変換する。
	//
	// 空を返したエントリは持ち出さない。「持ち出さない」と「危険だから
	// 拒否する」は別で、後者はこの関数を呼ぶ前に弾く。
	Rename func(entry string) string
	// Require は変換後に必ず含まれていなければならないエントリ名。
	//
	// 空なら確かめない。指定した名前が無ければ、1 バイトも残さず失敗する。
	// 中身の足りないアーカイブを渡すと、受け取った人は使おうとして
	// 初めて気づくことになる。
	Require string
}

/*
Remap はエントリ名を付け替えた新しいアーカイブを作る。

中身は 1 バイトも変えない。変えるのは名前と、持ち出すかどうかだけ。

安全の検証（絶対パス / .. / シンボリックリンク）は Rename より先に
通す。1 つでも違反があれば、出力を残さずアーカイブ全体を拒否する。
置き場所の判断に委ねると、危険なエントリを「持ち出さない」で
黙って通してしまう。
*/
func Remap(ctx context.Context, src, dest string, opts RemapOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Rename == nil {
		return errors.New("包み直しの規則が指定されていません")
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

	if err := remapEntries(ctx, r, out, opts); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("包み直しを確定できません: %w", err)
	}
	committed = true
	return nil
}

func remapEntries(ctx context.Context, r *zip.ReadCloser, out io.Writer, opts RemapOptions) error {
	w := zip.NewWriter(out)
	registerFastCompressor(w)

	buf := make([]byte, copyBufferSize)
	found := opts.Require == ""

	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return err
		}
		// 置き場所は問わず、展開先の外へ出る経路だけを塞ぐ。
		if _, _, err := normalizeEntryName(f.Name); err != nil {
			return err
		}
		// ディレクトリのエントリは持ち出さない。配下のファイルを
		// 展開するときに作られる。
		if isDirEntry(f.Name) {
			continue
		}

		name := opts.Rename(f.Name)
		if name == "" {
			continue
		}
		if err := copyOneEntry(w, f, name, buf); err != nil {
			return err
		}
		if name == opts.Require {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("%w: %s がありません", ErrRemapIncomplete, opts.Require)
	}
	return w.Close()
}
