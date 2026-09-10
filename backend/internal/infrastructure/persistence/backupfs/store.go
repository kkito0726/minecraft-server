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
		Level:       levelNameOf(manifest.LevelDir),
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

func (s *Store) pathOf(id backup.ID) string {
	return filepath.Join(s.cfg.BackupDir, id.String())
}

// levelNameOf は "data/world" から "world" を取り出す。
func levelNameOf(levelDir string) string {
	if levelDir == "" {
		return ""
	}
	return filepath.Base(levelDir)
}

var _ port.BackupStore = (*Store)(nil)
