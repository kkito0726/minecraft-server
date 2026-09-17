package backupctl

import (
	"context"

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
