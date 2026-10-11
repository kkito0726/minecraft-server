package backupctl

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
)

/*
ダウンロードは操作（Operation）にしない。

読むだけで data/ も .env も触らないため、「同時にひとつだけ」の枠を
消費させる理由がない。取得や復元の最中でも落とせる。作成途中の
アーカイブは一時ファイル（<名前>.zip.part-*）なので一覧に現れず、
中途半端なファイルを渡すことにもならない。
*/

// Exists はアーカイブが保管先にあるかを確かめ、ファイル名を返す。
//
// 受取券を出す前に呼ぶ。存在しない id で券を出すと、押した直後ではなく
// ダウンロードの途中で失敗し、理由が分かりにくくなる。
func (u *UseCase) Exists(ctx context.Context, backupID string) (string, error) {
	id, err := backup.NewID(backupID)
	if err != nil {
		return "", err
	}

	file, err := u.cfg.Store.Open(ctx, id)
	if err != nil {
		return "", err
	}
	// 中身は要らない。あることだけ確かめて閉じる。
	_ = file.Body.Close()
	return id.String(), nil
}

// OpenForDownload は保管済みのアーカイブを読み出し用に開く。
//
// 呼び出し側が Body を閉じる。
func (u *UseCase) OpenForDownload(ctx context.Context, backupID string) (port.ArchiveFile, error) {
	id, err := backup.NewID(backupID)
	if err != nil {
		return port.ArchiveFile{}, err
	}
	return u.cfg.Store.Open(ctx, id)
}

/*
OpenForSingleplayer は保管済みのアーカイブを、シングルプレイで読める形へ
包み直して開く。

Paper とバニラは、ワールドの中の .dat の置き場所が一部違う。Paper は
サーバーとして読むときに自動で吸収する（VanillaWorldMigration）が、
逆向きの機構はバニラに無い。そのまま saves/ に置くと、ワールド一覧には
出るのに「Overworld settings missing」で読み込みが止まる。

ダウンロードと同じく操作（Operation）にはしない。保管しているアーカイブを
読んで一時ファイルを書くだけで、data/ も .env も触らないため。
*/
func (u *UseCase) OpenForSingleplayer(
	ctx context.Context, backupID string,
) (port.ArchiveFile, error) {
	id, err := backup.NewID(backupID)
	if err != nil {
		return port.ArchiveFile{}, err
	}
	if err := u.ensureRewrapSpace(ctx, id); err != nil {
		return port.ArchiveFile{}, err
	}
	return u.cfg.Store.OpenVanilla(ctx, id)
}

/*
ensureRewrapSpace は包み直しに要る空きがあるかを確かめる。

包み直しは元のアーカイブとほぼ同じ大きさの一時ファイルを書く。持ち出さない
ぶんだけ小さくなるので、元の大きさを上限として見ておけば足りる。

始めてから埋めきると、サーバーそのものが書けなくなって止まる。Pi では
空き容量がそのままサーバーの生死に関わるので、書く前に断る。
*/
func (u *UseCase) ensureRewrapSpace(ctx context.Context, id backup.ID) error {
	// 大きさを知るために開くだけ。中身は読まない。
	file, err := u.cfg.Store.Open(ctx, id)
	if err != nil {
		return err
	}
	_ = file.Body.Close()

	available, err := u.cfg.Worlds.AvailableBytes(ctx)
	if err != nil {
		return err
	}
	if available < file.Info.SizeBytes {
		return fmt.Errorf("%w（必要 %d バイト、空き %d バイト）",
			port.ErrInsufficientSpace, file.Info.SizeBytes, available)
	}
	return nil
}
