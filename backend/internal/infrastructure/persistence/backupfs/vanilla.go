package backupfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

/*
シングルプレイへ持ち出すための包み直し。

保管しているのは Paper 形式のアーカイブだけで、シングルプレイ用は
渡すときにその場で作る。2 つ保管すると、同じワールドが二重に
場所を取る。Pi では空き容量がそのままサーバーの生死に関わる。
*/

// vanillaTempSuffix は包み直しの一時ファイルに付ける印。
//
// .zip で終わらない名前にしてある。backup.NewID が .zip を要求するため、
// List がこの名前を解釈できずに読み飛ばす。作りかけが一覧に出ない。
const vanillaTempSuffix = ".vanilla-"

/*
OpenVanilla は保管済みのアーカイブを、シングルプレイで読める形へ
包み直して開く。

保管しているアーカイブには手を触れない。包み直したものは一時
ファイルへ書き、開いた直後にディレクトリから外す。閉じた時点で
領域が返るので、ダウンロードが途中で切れても残骸が積まない。

Open と同じく Seek できる形で返す。シングルプレイ用の zip も
200MB 規模になるため、Range 要求に応えられる必要がある（REQ-415）。
*/
func (s *Store) OpenVanilla(ctx context.Context, id backup.ID) (port.ArchiveFile, error) {
	level, err := s.levelOf(ctx, id)
	if err != nil {
		return port.ArchiveFile{}, err
	}

	/*
		更新時刻は保管しているアーカイブのものを使う。

		この値は Last-Modified になり、Range で再開するときの If-Range の
		照合に使われる。包み直した一時ファイルの時刻を使うと、再開のために
		もう一度来た要求で包み直しが走って時刻が変わる。http.ServeContent は
		照合が外れた Range を無視して 200 で全体を返し直すので、200MB の
		転送が切れるたびに最初からやり直すことになる（REQ-135）。

		保管しているアーカイブは変わらないので、この時刻は安定する。
		包み直しの出力も決まっている（zip のエントリの時刻は書かない）ため、
		同じアーカイブからは毎回同じバイト列ができる。
	*/
	stored, err := os.Stat(s.pathOf(id))
	if err != nil {
		return port.ArchiveFile{}, fmt.Errorf("アーカイブを読めません (%s): %w", id, err)
	}

	tmp, err := s.vanillaTempPath(id)
	if err != nil {
		return port.ArchiveFile{}, err
	}

	err = archive.Remap(ctx, s.pathOf(id), tmp, archive.RemapOptions{
		Rename: func(entry string) string {
			return backup.VanillaEntryName(entry, level)
		},
		// level.dat が無ければワールドとして認識されない。
		// 中身の足りない zip を渡すと、saves/ に置いてから気づく。
		Require: level + "/" + "level.dat",
	})
	if err != nil {
		return port.ArchiveFile{}, fmt.Errorf(
			"シングルプレイ用に包み直せません (%s): %w", id, err)
	}

	return openAndUnlink(tmp, id, stored.ModTime())
}

// levelOf はアーカイブに入っているワールド名を、展開せずに読む。
func (s *Store) levelOf(ctx context.Context, id backup.ID) (string, error) {
	if _, err := os.Stat(s.pathOf(id)); errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", backup.ErrNotFound, id)
	}

	manifest, err := archive.Inspect(ctx, s.pathOf(id))
	if err != nil {
		return "", err
	}
	if manifest.LevelDir == "" {
		return "", fmt.Errorf(
			"%w: %s に含まれるワールドの名前を判定できません", backup.ErrNotFound, id)
	}
	return manifest.LevelDir, nil
}

// vanillaTempPath は包み直し先の名前を決める。
//
// 作らずに名前だけ返す。archive.Remap が O_EXCL で作るので、
// 万一ぶつかっても上書きにはならず失敗する。
func (s *Store) vanillaTempPath(id backup.ID) (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("一時ファイルの名前を決められません: %w", err)
	}
	return s.pathOf(id) + vanillaTempSuffix + hex.EncodeToString(buf[:]), nil
}

/*
openAndUnlink はファイルを開いてから、ディレクトリから名前を外す。

開いている間は内容が生きていて、閉じた時点で領域が返る。明示的な
後始末が要らないので、ダウンロードが中断しても、プロセスが落ちても、
消し忘れた一時ファイルが保管先に積まない。
*/
func openAndUnlink(path string, id backup.ID, modTime time.Time) (port.ArchiveFile, error) {
	f, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return port.ArchiveFile{}, fmt.Errorf("包み直したアーカイブを開けません: %w", err)
	}

	// 名前を外すのは開いた直後。先に外すと開けなくなり、
	// 後に回すと失敗した経路で残る。
	if err := os.Remove(path); err != nil {
		_ = f.Close()
		return port.ArchiveFile{}, fmt.Errorf("一時ファイルを外せません: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return port.ArchiveFile{}, fmt.Errorf("包み直したアーカイブを読めません: %w", err)
	}

	// 大きさは包み直したものから。時刻は保管しているものから。
	return port.ArchiveFile{
		Body: f,
		Info: port.StoredBackup{ID: id, SizeBytes: info.Size(), CreatedAt: modTime},
	}, nil
}
