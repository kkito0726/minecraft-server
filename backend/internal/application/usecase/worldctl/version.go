package worldctl

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/versionsync"
)

// keyVersion はサーバーの版。規則は versionsync が持つ。
const keyVersion = versionsync.KeyVersion

// 版の切り替えに関わるエラー。規則そのものは versionsync にある。
//
// 復元（backupctl）でも同じ判断をするため、実体はそちらへ寄せてある。
// ここに残してあるのは、rpc がこの名前で状態コードへ対応づけているため。
var (
	// ErrVersionUnavailable は Paper がその版を配っていないことを表す。
	ErrVersionUnavailable = versionsync.ErrVersionUnavailable
	// ErrCatalogUnavailable は版の一覧が分からないことを表す。
	ErrCatalogUnavailable = versionsync.ErrCatalogUnavailable
)

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

	// 作成では一覧が取れないことを許さない。無い版で作ると
	// サーバーが起動せず、ワールドも出来上がらない。
	if _, err := u.stableVersions(ctx); err != nil {
		return fmt.Errorf("%w。現在の版 %s 以外では作れません: %w",
			ErrCatalogUnavailable, current, err)
	}
	if err := versionsync.CheckAvailable(ctx, u.cfg.Catalog, current, version); err != nil {
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
	if err != nil {
		return err
	}
	return versionsync.CheckAvailable(ctx, u.cfg.Catalog, current, version)
}

func (u *UseCase) stableVersions(ctx context.Context) ([]string, error) {
	if u.cfg.Catalog == nil {
		return nil, ErrCatalogUnavailable
	}
	return u.cfg.Catalog.StableVersions(ctx)
}

// currentVersion は .env の MC_VERSION を読む。
func (u *UseCase) currentVersion(ctx context.Context) (string, error) {
	return versionsync.Current(ctx, u.cfg.Config)
}

func currentOnly(current string) []string {
	if current == "" {
		return nil
	}
	return []string{current}
}
