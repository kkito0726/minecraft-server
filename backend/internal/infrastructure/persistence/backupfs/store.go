// Package backupfs はバックアップのアーカイブをファイルシステムに保管する。
//
// data/ はバインドマウントなので、ホストから直接 zip / unzip できる。
// docs/raspberry-pi.md が「名前付きボリュームへの移行はバックアップ手順を
// 壊すので採用しない」と判断した結果、この単純さが保たれている。
package backupfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/archive"
)

// dataDirName はアーカイブ内のエントリの接頭辞。
// docs/backup-restore.md の zip -qr が data/... を作るのに合わせる。
const dataDirName = "data"

// Config は Store の設定。
type Config struct {
	// ProjectDir は compose.yaml と data/ があるディレクトリ。
	ProjectDir string
	// BackupDir はアーカイブの保管先。
	BackupDir string
}

// Store はアーカイブの保管先。
type Store struct {
	cfg Config
}

// New は Store を作る。
func New(cfg Config) (*Store, error) {
	if cfg.ProjectDir == "" || cfg.BackupDir == "" {
		return nil, errors.New("プロジェクトディレクトリと保管先が必要です")
	}
	return &Store{cfg: cfg}, nil
}

// Directory は保管先のパスを返す。
func (s *Store) Directory() string { return s.cfg.BackupDir }

// List は保管済みのアーカイブを返す。
//
// 解釈できないファイルは読み飛ばす。作成途中の一時ファイル
// （<名前>.zip.part-*）やディレクトリ、人が置いたメモが混ざっていても
// 一覧そのものは成立させる。1 つの異物で画面が空になってはならない。
func (s *Store) List(context.Context) ([]port.StoredBackup, error) {
	entries, err := os.ReadDir(s.cfg.BackupDir)
	if errors.Is(err, os.ErrNotExist) {
		// まだ 1 度も取っていない状態。0 件であってエラーではない。
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("保管先を読めません (%s): %w", s.cfg.BackupDir, err)
	}

	var out []port.StoredBackup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, err := backup.NewID(e.Name())
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, port.StoredBackup{
			ID:        id,
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	return out, nil
}

// Create はアーカイブを作る。
//
// 対象は backup.ExtraPaths() とワールドの網羅列挙。ここに無いものは
// 入れない。とくに server.properties は rcon.password を平文で持つため、
// 含めるとバックアップを渡した相手にサーバーの操作権を渡すことになる。
func (s *Store) Create(
	ctx context.Context, id backup.ID, level world.Name, progress port.Progress,
) (port.StoredBackup, error) {
	if err := os.MkdirAll(s.cfg.BackupDir, 0o755); err != nil {
		return port.StoredBackup{}, fmt.Errorf("保管先を作れません (%s): %w", s.cfg.BackupDir, err)
	}

	dest := s.pathOf(id)
	if err := archive.Create(ctx, dest, s.sourcesFor(level), archive.Progress(progress)); err != nil {
		return port.StoredBackup{}, err
	}

	info, err := os.Stat(dest)
	if err != nil {
		return port.StoredBackup{}, fmt.Errorf("作成したアーカイブを読めません: %w", err)
	}
	return port.StoredBackup{ID: id, SizeBytes: info.Size(), CreatedAt: info.ModTime()}, nil
}

// sourcesFor はアーカイブに含める対象を組み立てる。
//
// ワールドは必須。実在しなければ archive.Create が失敗し、
// 中身の無いアーカイブが「バックアップが取れた」として残ることはない。
// 一方 plugins や spigot.yml は環境によって存在しないため、飛ばす。
func (s *Store) sourcesFor(level world.Name) []archive.Source {
	sources := []archive.Source{{
		Root:         s.cfg.ProjectDir,
		Path:         filepath.Join(dataDirName, level.String()),
		ExcludeNames: backup.ExcludedNames(),
	}}

	for _, extra := range backup.ExtraPaths() {
		rel := filepath.Join(dataDirName, extra)
		if _, err := os.Stat(filepath.Join(s.cfg.ProjectDir, rel)); err != nil {
			continue
		}
		sources = append(sources, archive.Source{Root: s.cfg.ProjectDir, Path: rel})
	}
	return sources
}

// Delete はアーカイブを削除し、解放されたバイト数を返す。
func (s *Store) Delete(_ context.Context, id backup.ID) (int64, error) {
	path := s.pathOf(id)

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("%w: %s", backup.ErrNotFound, id)
	}
	if err != nil {
		return 0, fmt.Errorf("アーカイブを読めません (%s): %w", id, err)
	}
	if err := os.Remove(path); err != nil {
		return 0, fmt.Errorf("アーカイブを削除できません (%s): %w", id, err)
	}
	return info.Size(), nil
}

// Inspect はアーカイブの構成を、展開せずに読む。
func (s *Store) Inspect(ctx context.Context, id backup.ID) (port.ArchiveInfo, error) {
	manifest, err := archive.Inspect(ctx, s.pathOf(id))
	if err != nil {
		return port.ArchiveInfo{}, err
	}

	return port.ArchiveInfo{
		Level:       manifest.LevelDir,
		EntryRoots:  manifest.Roots,
		TotalBytes:  manifest.TotalBytes,
		HasLevelDat: manifest.LevelDatEntry != "",
	}, nil
}

// OpenLevelDat はアーカイブ内の level.dat を、展開せずに開く。
//
// バージョンの判定に必要なのは数十バイトだけなので、
// 30MB のワールドを展開してから読むことはしない。
func (s *Store) OpenLevelDat(ctx context.Context, id backup.ID) (io.ReadCloser, error) {
	manifest, err := archive.Inspect(ctx, s.pathOf(id))
	if err != nil {
		return nil, err
	}
	if manifest.LevelDatEntry == "" {
		return nil, fmt.Errorf("%w: %s に level.dat がありません", backup.ErrNotFound, id)
	}
	return archive.OpenEntry(ctx, s.pathOf(id), manifest.LevelDatEntry)
}

// Extract はアーカイブを ProjectDir へ展開する。
//
// エントリ名は data/... なので、展開先の基点はプロジェクトディレクトリになる。
// 危険なエントリが 1 つでもあれば archive.Extract が 1 バイトも書かずに拒否する。
//
// rewriteLevel が空でなければ、アーカイブ内のワールド名をその名前へ
// 書き換えて展開する。これが無いと、利用者が別のワールドへ切り替えている
// 状態でアーカイブのワールドが戻るだけになり、稼働中のワールドは
// 変化しないまま「復元したのに何も起きない」ことになる。
func (s *Store) Extract(
	ctx context.Context, id backup.ID, rewriteLevel string, progress port.Progress,
) error {
	opts, err := s.rewriteOptions(ctx, id, rewriteLevel)
	if err != nil {
		return err
	}
	return archive.Extract(ctx, s.pathOf(id), s.cfg.ProjectDir, opts, archive.Progress(progress))
}

func (s *Store) rewriteOptions(
	ctx context.Context, id backup.ID, rewriteLevel string,
) (archive.ExtractOptions, error) {
	if rewriteLevel == "" {
		return archive.ExtractOptions{}, nil
	}

	manifest, err := archive.Inspect(ctx, s.pathOf(id))
	if err != nil {
		return archive.ExtractOptions{}, err
	}
	if manifest.LevelDir == "" {
		return archive.ExtractOptions{}, fmt.Errorf(
			"%s に含まれるワールドの名前を判定できないため、復元先を書き換えられません", id)
	}

	if manifest.LevelDir == rewriteLevel {
		return archive.ExtractOptions{}, nil
	}

	// Manifest.LevelDir は data/ を除いたワールド名なので、
	// エントリ名に合わせて data/ を付け直す。
	// 末尾に / を付けるのは data/world が data/worldbackup に
	// 誤って一致するのを防ぐため。
	from := dataDirName + "/" + manifest.LevelDir + "/"
	to := dataDirName + "/" + rewriteLevel + "/"
	return archive.ExtractOptions{RewritePrefix: map[string]string{from: to}}, nil
}

func (s *Store) pathOf(id backup.ID) string {
	return filepath.Join(s.cfg.BackupDir, id.String())
}

var _ port.BackupStore = (*Store)(nil)
