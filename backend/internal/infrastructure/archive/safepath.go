package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// ErrUnsafeEntry はアーカイブのエントリが展開先の外を指していることを表す。
var ErrUnsafeEntry = errors.New("アーカイブに安全でないエントリがあります")

// allowedRoot は展開してよい接頭辞。
//
// アーカイブに含まれるのは data/ 配下だけなので、これ以外を指すエントリは
// 意図しないもの（あるいは攻撃）として扱う。
const allowedRoot = "data/"

// dataDirName は allowedRoot から末尾の / を除いたもの。
// ディレクトリのエントリの判定に使う。
const dataDirName = "data"

// safeEntryName はエントリ名を検証し、正規化した名前を返す。
//
// zip のエントリ名は攻撃者が自由に決められる。素直に filepath.Join すると
// "../../etc/passwd" のようなエントリで展開先の外に書き込める（zip-slip）。
//
// 展開を始める前にこの検証を通し、1 つでも違反があればアーカイブ全体を
// 拒否する。途中まで書いてから気づくと中途半端なファイルが残る。
func safeEntryName(name string) (string, error) {
	cleaned, normalized, err := normalizeEntryName(name)
	if err != nil {
		return "", err
	}

	// data/ そのものを指すディレクトリのエントリは通す。
	//
	// zip -r out.zip data のように data ごと固めると、アーカイブに
	// "data/" というエントリが入る。path.Clean が末尾の / を落とすため
	// "data" になり、そのままでは「data/ 配下ではない」と判定される。
	// 1 つでも違反があればアーカイブ全体を拒否する作りなので、
	// これを弾くと手で作った zip が丸ごと復元できなくなる。
	//
	// 末尾が / であることを条件にするのは、ディレクトリとして
	// 書かれていない "data" は data/ の外にある別のファイルだから。
	if cleaned == dataDirName && strings.HasSuffix(normalized, "/") {
		return cleaned, nil
	}

	if !strings.HasPrefix(cleaned, allowedRoot) {
		return "", fmt.Errorf("%w: %q は %s 配下ではありません", ErrUnsafeEntry, name, allowedRoot)
	}
	return cleaned, nil
}

/*
normalizeEntryName は展開先の**外**へ出る経路だけを塞ぐ。

置き場所（data/ 配下であること）は問わない。持ち込まれたアーカイブは
まだ data/ 配下に無く、中身を見て判断する段階ではここを要求できない
ため、安全の検証と置き場所の検証を分けてある。

戻り値は正規化した名前と、区切りを / に揃えただけの名前。
後者はディレクトリのエントリ（末尾が /）の判定に使う。
*/
func normalizeEntryName(name string) (cleaned, normalized string, err error) {
	if name == "" {
		return "", "", fmt.Errorf("%w: エントリ名が空です", ErrUnsafeEntry)
	}

	// zip の仕様上の区切りは / だが、Windows で作られたアーカイブは
	// \ を含むことがある。判定前に統一する。
	normalized = strings.ReplaceAll(name, `\`, "/")

	// ドライブレターつきの絶対パス（C:\... など）
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", "", fmt.Errorf("%w: %q は絶対パスです", ErrUnsafeEntry, name)
	}
	if strings.HasPrefix(normalized, "/") {
		return "", "", fmt.Errorf("%w: %q は絶対パスです", ErrUnsafeEntry, name)
	}

	// path.Clean が ".." を解決する。解決後にまだ ".." で始まるなら脱出している。
	cleaned = path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", "", fmt.Errorf("%w: %q が展開先の外を指しています", ErrUnsafeEntry, name)
	}
	return cleaned, normalized, nil
}

// checkMode はエントリの種別を検証する。
//
// シンボリックリンクを展開すると、リンク先を経由して data/ の外へ書ける。
// 通常のファイルとディレクトリだけを許可する。
func checkMode(name string, mode fs.FileMode) error {
	switch {
	case mode&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %q はシンボリックリンクです", ErrUnsafeEntry, name)
	case mode&fs.ModeDevice != 0, mode&fs.ModeNamedPipe != 0, mode&fs.ModeSocket != 0:
		return fmt.Errorf("%w: %q は通常のファイルではありません", ErrUnsafeEntry, name)
	default:
		return nil
	}
}
