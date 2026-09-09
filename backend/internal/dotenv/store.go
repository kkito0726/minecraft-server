package dotenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrConflict は、読み込んでから書き戻すまでの間に .env が
// 外部から変更されていたことを表す。
//
// README は vi .env による手編集を正規の手順として案内しているため、
// 人間の編集を黙って上書きしてはいけない。
var ErrConflict = errors.New(".env が外部から変更されました")

// Store は .env の読み書き。
type Store interface {
	Load(ctx context.Context) (*Snapshot, error)
	Save(ctx context.Context, s *Snapshot) error
}

// Snapshot は読み込んだ時点の内容と、その時点のファイルの状態。
// 不変で、WithValue は新しい Snapshot を返す。
type Snapshot struct {
	file     *File
	loadedAt time.Time
	// size と modTime は楽観ロックの判定に使う。
	size    int64
	modTime time.Time
}

// File は内容を返す。
func (s *Snapshot) File() *File { return s.file }

// LoadedAt は読み込んだ時刻を返す。
func (s *Snapshot) LoadedAt() time.Time { return s.loadedAt }

// WithValue はキーの値を差し替えた新しい Snapshot を返す。
// 楽観ロックの判定材料（読み込み時点のサイズと更新時刻）は引き継ぐ。
func (s *Snapshot) WithValue(key, value string) *Snapshot {
	return &Snapshot{
		file:     s.file.With(key, value),
		loadedAt: s.loadedAt,
		size:     s.size,
		modTime:  s.modTime,
	}
}

// WithoutValue はキーを取り除いた新しい Snapshot を返す。
func (s *Snapshot) WithoutValue(key string) *Snapshot {
	return &Snapshot{
		file:     s.file.Without(key),
		loadedAt: s.loadedAt,
		size:     s.size,
		modTime:  s.modTime,
	}
}

// FileStore はファイルシステム上の .env を読み書きする。
type FileStore struct {
	path string
}

// NewFileStore は指定したパスの .env を扱う Store を作る。
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Path は対象のパスを返す。
func (s *FileStore) Path() string { return s.path }

// Load は .env を読む。
//
// 操作のたびに毎回ディスクから読む。プロセス内にキャッシュを持つと、
// 人間が vi .env で編集した内容を知らないまま上書きすることになる。
func (s *FileStore) Load(ctx context.Context) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf(".env を開けません (%s): %w", s.path, err)
	}
	// 読み取り専用なので Close の失敗は結果に影響しない
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf(".env の状態を取得できません: %w", err)
	}

	parsed, err := Parse(f)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		file:     parsed,
		loadedAt: time.Now(),
		size:     info.Size(),
		modTime:  info.ModTime(),
	}, nil
}

// Save は .env を書き戻す。
//
// 読み込み時点からファイルが変わっていれば ErrConflict を返して書き込まない。
// 書き込みは同じディレクトリの一時ファイル経由で、元と同じパーミッションで行う
// （0644 の一時ファイルから rename すると 0600 の .env の権限が緩む）。
func (s *FileStore) Save(ctx context.Context, snap *Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	perm, err := s.checkUnmodified(snap)
	if err != nil {
		return err
	}
	return s.writeAtomic(snap.file, perm)
}

// checkUnmodified は読み込み時点からファイルが変わっていないことを確かめ、
// 現在のパーミッションを返す。
func (s *FileStore) checkUnmodified(snap *Snapshot) (os.FileMode, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, fmt.Errorf(".env の状態を取得できません: %w", err)
	}
	if info.Size() != snap.size || !info.ModTime().Equal(snap.modTime) {
		return 0, fmt.Errorf("%w (%s)。もう一度読み込んでから操作してください", ErrConflict, s.path)
	}
	return info.Mode().Perm(), nil
}

// writeAtomic は一時ファイルへ書いてから rename する。
// 途中で電源が落ちても、元の .env が中途半端な内容になることはない。
func (s *FileStore) writeAtomic(f *File, perm os.FileMode) (err error) {
	dir := filepath.Dir(s.path)

	tmp, err := os.CreateTemp(dir, ".env.tmp-*")
	if err != nil {
		return fmt.Errorf("一時ファイルを作れません: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			// 失敗経路の後始末。ここでのエラーは元のエラーを覆い隠すだけなので無視する
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	// CreateTemp は 0600 で作る。元のパーミッションに合わせる。
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("一時ファイルのパーミッションを設定できません: %w", err)
	}
	if _, err = f.WriteTo(tmp); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("一時ファイルを同期できません: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("一時ファイルを閉じられません: %w", err)
	}
	if err = os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf(".env を置き換えられません: %w", err)
	}

	return syncDir(dir)
}

// syncDir は rename をディスクに確定させる。
// これを省くと、電源断で .env が消えたように見えることがある。
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("ディレクトリを開けません: %w", err)
	}
	defer func() { _ = d.Close() }()

	if err := d.Sync(); err != nil {
		// ディレクトリの fsync に対応しない環境がある。書き込み自体は成功しているので
		// 失敗として扱わない。
		if errors.Is(err, os.ErrInvalid) {
			return nil
		}
		return fmt.Errorf("ディレクトリを同期できません: %w", err)
	}
	return nil
}

// 実装がインターフェースを満たすことをコンパイル時に確かめる。
var _ Store = (*FileStore)(nil)
var _ io.WriterTo = (*File)(nil)
