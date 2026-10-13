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

// allowedRoot は LayoutUnderData のアーカイブが要求する接頭辞。
//
// この形のアーカイブは展開の基点がプロジェクトディレクトリになるので、
// data/ の外を指すエントリが混ざっていると compose.yaml や .env と
// 並ぶ位置に書けてしまう。だから接頭辞を要求する。
//
// LayoutWorldRoot では要求しない。基点が data/ なので、どんな名前でも
// その外には出ない（Layout のコメントを参照）。
const allowedRoot = "data/"

// dataDirName は allowedRoot から末尾の / を除いたもの。
// ディレクトリのエントリの判定に使う。
const dataDirName = "data"

/*
safeEntryName はエントリ名を検証し、正規化した名前を返す。

zip のエントリ名は攻撃者が自由に決められる。素直に filepath.Join すると
"../../etc/passwd" のようなエントリで展開先の外に書き込める（zip-slip）。

展開を始める前にこの検証を通し、1 つでも違反があればアーカイブ全体を
拒否する。途中まで書いてから気づくと中途半端なファイルが残る。

検証は 2 つに分かれている。

  - 安全の検証（normalizeEntryName）— 展開先の外へ出る経路を塞ぐ。
    形に関わらず常に効く
  - 置き場所の検証（checkPlacement）— 形に合っているかを見る。
    LayoutWorldRoot では展開の基点そのものが置き場所を保証するので、
    接頭辞を要求しない
*/
func safeEntryName(name string, layout Layout) (string, error) {
	cleaned, normalized, err := normalizeEntryName(name)
	if err != nil {
		return "", err
	}
	if err := checkPlacementNormalized(name, cleaned, normalized, layout); err != nil {
		return "", err
	}
	return cleaned, nil
}

// checkPlacement は正規化済みの名前が形に合っているかを見る。
func checkPlacement(cleaned string, layout Layout) error {
	return checkPlacementNormalized(cleaned, cleaned, cleaned, layout)
}

func checkPlacementNormalized(raw, cleaned, normalized string, layout Layout) error {
	prefix := layout.requiredPrefix()
	if prefix == "" {
		// 置き場所の制限が無い形。基点が data/ なので外へは出ない。
		return nil
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
		return nil
	}

	if !strings.HasPrefix(cleaned, prefix) {
		return fmt.Errorf("%w: %q は %s 配下ではありません", ErrUnsafeEntry, raw, prefix)
	}
	return nil
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

	normalized = toSlash(name)

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

// toSlash は区切りを / に揃える。
//
// zip の仕様上の区切りは / だが、Windows で作られたアーカイブは
// \ を含むことがある。名前の判定はすべてこれを通してから行う。
func toSlash(name string) string {
	return strings.ReplaceAll(name, `\`, "/")
}

// isDirEntry はエントリがディレクトリを表すかを返す。
//
// 生の名前で末尾の / を見ると、`data\world\` のような Windows 製の
// エントリをファイルとして扱う。ディレクトリの位置に 0 バイトの
// ファイルが書かれ、その配下を作れずに展開が途中で失敗する。
//
// 展開・読み取り・包み直しの 4 つの入口が同じ判定を要るので、
// 各自で書かずにここへ寄せる。
func isDirEntry(name string) bool {
	return strings.HasSuffix(toSlash(name), "/")
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
