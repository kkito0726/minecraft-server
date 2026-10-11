/*
Package versionsync はサーバーの版をワールドに合わせる規則。

ワールドの版上げは片道で、新しい版で開いたワールドは古いサーバーでは
二度と開けない。だから「ワールドを切り替えたらサーバーの版もそれに
合わせる」のが既定の振る舞いになっている。合わせないと、

  - 古いワールドは今の版で開かれて勝手に上がる（元に戻せない）
  - 新しいワールドは古い版では開けず、サーバーが起動しない

この規則が要る場所は 2 つある。ワールドの切り替え（worldctl）と
バックアップの復元（backupctl）で、どちらも「別のワールドを
開き直す」操作である。片方だけ直すと、もう片方が静かに腐る。
*/
package versionsync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// KeyVersion はサーバーの版。
//
// ワールドの版上げは片道なので、これを書くのは次の 3 つに限る。
//   - ワールドの作成時（生成に使う版を選ぶ）
//   - 切り替え時（切り替え先のワールドが最後に開かれた版に合わせる）
//   - 復元時（復元したワールドが最後に開かれた版に合わせる）
//
// どれも「ワールドを今より新しい版で開く」ことは起こさない。版を上げる
// 操作は画面には置かず、手順書（runbook）の手作業に残してある。
const KeyVersion = "MC_VERSION"

// ErrVersionUnavailable は Paper がその版を配っていないことを表す。
//
// 書くとサーバーが起動しない。現在の版で開くとワールドが勝手に上がる。
// どちらも避けるため、止める前に断る。
var ErrVersionUnavailable = errors.New("この版は Paper で配布されていません")

// ErrCatalogUnavailable は版の一覧が分からないことを表す。
//
// 一覧が無いと、書こうとしている版が起動するかを確かめられない。
var ErrCatalogUnavailable = errors.New("版の一覧を取得できません")

/*
Follow はサーバーの版を、開き直すワールドが最後に開かれた版に合わせる。

戻り値の 2 つめは利用者へ残す注意書き。版が変わるときだけ空でなくなる。
クライアント側も合わせないと繋げないので、黙って変えてはならない。

読めないとき（level.dat が無い、スナップショットで開いた）は触らない。
Paper はスナップショットを配らないので、書いても起動しなくなるだけ。
*/
func Follow(
	current, next port.ConfigSnapshot, level port.LevelSettings,
) (port.ConfigSnapshot, string) {
	if !level.HasVersion {
		return next, ""
	}
	before, _ := current.Get(KeyVersion)
	if strings.TrimSpace(before) == level.Version {
		return next, ""
	}
	return next.With(KeyVersion, level.Version), fmt.Sprintf(
		"サーバーの版を %s から %s に切り替えます。遊ぶ人はクライアントを %s にしてください",
		strings.TrimSpace(before), level.Version, level.Version)
}

/*
CheckAvailable は want の版で起動できるかを、止める前に確かめる。

一覧が取れないとき（Pi がオフライン）は通す。itzg のイメージがその版の
jar を既に持っていれば起動できるし、起動できなければ元のワールドへ
切り替え直せば版も戻る。ここで止めてしまうと、オフラインの間は一切
切り替えられない。

catalog が nil のときも同じ扱いにする。一覧を持たない構成で切り替えを
塞ぐ理由がない。
*/
func CheckAvailable(
	ctx context.Context, catalog port.VersionCatalog, current, want string,
) error {
	if want == "" || want == current {
		return nil
	}
	if catalog == nil {
		return nil
	}
	versions, err := catalog.StableVersions(ctx)
	if err != nil {
		return nil
	}
	if !slices.Contains(versions, want) {
		return fmt.Errorf(
			"%w: %s。このワールドを開くには版を上げる必要があり、上げると元に戻せません。"+
				"上げてよい場合は runbook の手順で MC_VERSION を書き換えてください",
			ErrVersionUnavailable, want)
	}
	return nil
}

// Current は .env の MC_VERSION を読む。
func Current(ctx context.Context, cfg port.ServerConfig) (string, error) {
	snapshot, err := cfg.Load(ctx)
	if err != nil {
		return "", err
	}
	v, _ := snapshot.Get(KeyVersion)
	return strings.TrimSpace(v), nil
}
