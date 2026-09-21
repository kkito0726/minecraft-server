package worldctl

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// keyVersion はサーバーの版。
//
// ワールドの版上げは片道なので、これを書くのは次の 2 つに限る。
//   - ワールドの作成時（生成に使う版を選ぶ）
//   - 切り替え時（切り替え先のワールドが最後に開かれた版に合わせる）
//
// どちらも「ワールドを今より新しい版で開く」ことは起こさない。版を上げる
// 操作は画面には置かず、手順書（runbook）の手作業に残してある。
const keyVersion = "MC_VERSION"

// ErrVersionUnavailable は Paper がその版を配っていないことを表す。
//
// 書くとサーバーが起動しない。現在の版で開くとワールドが勝手に上がる。
// どちらも避けるため、止める前に断る。
var ErrVersionUnavailable = errors.New("この版は Paper で配布されていません")

// ErrCatalogUnavailable は版の一覧が分からないことを表す。
//
// 一覧が無いと、書こうとしている版が起動するかを確かめられない。
// 作成では現在の版（今まさに動いている版）だけを許す。
var ErrCatalogUnavailable = errors.New("版の一覧を取得できません")

// VersionListing は作れる版の一覧。
type VersionListing struct {
	// Versions は新しい順。一覧が取れないときは Current だけ。
	Versions []string
	Current  string
	// Available は Paper の一覧を取れたか。
	Available         bool
	UnavailableReason string
}

// ListVersions は作れる版の一覧を返す。
//
// 一覧が取れなくても失敗にはしない。ダイアログそのものが開けなくなると、
// 現在の版で作ることすらできなくなるため。
func (u *UseCase) ListVersions(ctx context.Context) (VersionListing, error) {
	current, err := u.currentVersion(ctx)
	if err != nil {
		return VersionListing{}, err
	}

	versions, err := u.stableVersions(ctx)
	if err != nil {
		return VersionListing{
			Versions:          currentOnly(current),
			Current:           current,
			UnavailableReason: err.Error(),
		}, nil
	}
	return VersionListing{Versions: versions, Current: current, Available: true}, nil
}

// validateCreateVersion は作成時に指定された版を確かめる。
// サーバーを止める前に呼ぶ。止めてから断ると、落ちたまま残る。
func (u *UseCase) validateCreateVersion(ctx context.Context, version string) error {
	if version == "" {
		return nil
	}
	current, err := u.currentVersion(ctx)
	if err != nil {
		return err
	}
	if version == current {
		return nil
	}

	versions, err := u.stableVersions(ctx)
	if err != nil {
		return fmt.Errorf("%w。現在の版 %s 以外では作れません: %w",
			ErrCatalogUnavailable, current, err)
	}
	if !slices.Contains(versions, version) {
		return fmt.Errorf("%w: %s", ErrVersionUnavailable, version)
	}
	return nil
}

// checkSwitchVersion は切り替え先のワールドの版で起動できるかを確かめる。
//
// 一覧が取れないとき（オフライン）は通す。itzg のイメージがその版の jar を
// 既に持っていれば起動できるし、起動できなければ元のワールドへ切り替え
// 直せば版も戻る。止めてしまうと、オフラインの間は一切切り替えられない。
func (u *UseCase) checkSwitchVersion(ctx context.Context, version string) error {
	current, err := u.currentVersion(ctx)
	if err != nil || version == current {
		return err
	}
	versions, err := u.stableVersions(ctx)
	if err != nil {
		return nil
	}
	if !slices.Contains(versions, version) {
		return fmt.Errorf(
			"%w: %s。このワールドを開くには版を上げる必要があり、上げると元に戻せません。"+
				"上げてよい場合は runbook の手順で MC_VERSION を書き換えてください",
			ErrVersionUnavailable, version)
	}
	return nil
}

func (u *UseCase) stableVersions(ctx context.Context) ([]string, error) {
	if u.cfg.Catalog == nil {
		return nil, ErrCatalogUnavailable
	}
	return u.cfg.Catalog.StableVersions(ctx)
}

// currentVersion は .env の MC_VERSION を読む。
func (u *UseCase) currentVersion(ctx context.Context) (string, error) {
	snapshot, err := u.cfg.Config.Load(ctx)
	if err != nil {
		return "", err
	}
	v, _ := snapshot.Get(keyVersion)
	return strings.TrimSpace(v), nil
}

func currentOnly(current string) []string {
	if current == "" {
		return nil
	}
	return []string{current}
}
