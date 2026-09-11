package backupfs

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

// ErrTooLarge は受け取れる大きさを超えたことを表す。
var ErrTooLarge = errors.New("アーカイブが大きすぎます")

/*
Stage は受け取ったバイト列を保管先の一時ファイルへ書き、中身を調べる。

保管先と同じディレクトリに書くのは、確定のときに同一ファイルシステム内の
rename で済ませるため。別の場所に置くとコピーになり、30MB のワールドで
ディスクを二度書くことになる。

一時ファイルには .zip を付けない。List は「.zip で終わるもの」だけを
アーカイブとみなすので、拡張子を付けないことで取り込みの途中に
一覧へ中途半端なものが現れなくなる。
*/
func (s *Store) Stage(
	ctx context.Context, src io.Reader, maxBytes int64,
) (port.ArchiveImport, error) {
	if err := os.MkdirAll(s.cfg.BackupDir, 0o755); err != nil {
		return nil, fmt.Errorf("保管先を作れません (%s): %w", s.cfg.BackupDir, err)
	}

	tmp, err := os.CreateTemp(s.cfg.BackupDir, ".part-import-*")
	if err != nil {
		return nil, fmt.Errorf("一時ファイルを作れません: %w", err)
	}
	path := tmp.Name()

	if err := writeLimited(ctx, tmp, src, maxBytes); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("一時ファイルを閉じられません: %w", err)
	}

	survey, err := archive.Survey(ctx, path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	return &stagedArchive{store: s, path: path, survey: survey}, nil
}

// writeLimited は上限つきで書き出し、確実にディスクへ落とす。
func writeLimited(ctx context.Context, dst *os.File, src io.Reader, maxBytes int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// 上限より 1 バイト多く読む。ちょうど上限で終わったのか、
	// 超えたのかを区別するため。
	written, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return fmt.Errorf("受け取れません: %w", err)
	}
	if written > maxBytes {
		return fmt.Errorf("%w（上限 %d バイト）", ErrTooLarge, maxBytes)
	}
	if written == 0 {
		return fmt.Errorf("%w: 中身がありません", backup.ErrNotImportable)
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("書き込みを確定できません: %w", err)
	}
	return nil
}

// stagedArchive は取り込み途中のアーカイブ。
type stagedArchive struct {
	store  *Store
	path   string
	survey archive.SurveyResult
	// adopted が真なら一時ファイルは既に無い。
	adopted bool
}

func (a *stagedArchive) LevelDatEntries() []string { return a.survey.LevelDatEntries }
func (a *stagedArchive) TotalBytes() int64         { return a.survey.TotalBytes }

func (a *stagedArchive) OpenEntry(ctx context.Context, entry string) (io.ReadCloser, error) {
	return openZipEntry(ctx, a.path, entry)
}

/*
Adopt は一時ファイルを正式な名前で保管先に置く。

prefix が空なら rename するだけ。空でなければ data/ 配下へ包み直した
新しいアーカイブを作ってから置く。包み直しに失敗しても、元の一時
ファイルはそのまま残るので中途半端な結果にはならない。
*/
func (a *stagedArchive) Adopt(
	ctx context.Context, id backup.ID, prefix string,
) (port.StoredBackup, error) {
	dest := a.store.pathOf(id)
	// 同じ名前が既にあれば上書きしない。取り込みで既存のバックアップを
	// 失うことがあってはならない。
	if _, err := os.Stat(dest); err == nil {
		return port.StoredBackup{}, fmt.Errorf("%w: %s", os.ErrExist, id)
	}

	source := a.path
	if prefix != "" {
		wrapped := source + ".wrapped"
		if err := archive.Rewrap(ctx, source, wrapped, prefix); err != nil {
			return port.StoredBackup{}, err
		}
		defer func() { _ = os.Remove(wrapped) }()
		source = wrapped
	}

	if err := os.Rename(source, dest); err != nil {
		return port.StoredBackup{}, fmt.Errorf("保管先へ置けません: %w", err)
	}
	a.adopted = true
	// 包み直した場合、元の一時ファイルはまだ残っている。
	if source != a.path {
		_ = os.Remove(a.path)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return port.StoredBackup{}, fmt.Errorf("置いたアーカイブを読めません: %w", err)
	}
	return port.StoredBackup{ID: id, SizeBytes: info.Size(), CreatedAt: info.ModTime()}, nil
}

func (a *stagedArchive) Discard() {
	if a.adopted {
		return
	}
	_ = os.Remove(a.path)
}

// openZipEntry は zip 内の 1 エントリを、全体を展開せずに開く。
func openZipEntry(ctx context.Context, src, entry string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, fmt.Errorf("アーカイブを開けません: %w", err)
	}
	for _, f := range r.File {
		if filepath.ToSlash(f.Name) != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("エントリを開けません (%s): %w", entry, err)
		}
		return &entryReader{ReadCloser: rc, zip: r}, nil
	}
	_ = r.Close()
	return nil, fmt.Errorf("%w: %s", backup.ErrNotFound, entry)
}

// entryReader はエントリを閉じるときにアーカイブも閉じる。
type entryReader struct {
	io.ReadCloser
	zip *zip.ReadCloser
}

func (e *entryReader) Close() error {
	err := e.ReadCloser.Close()
	if zerr := e.zip.Close(); err == nil {
		err = zerr
	}
	return err
}
