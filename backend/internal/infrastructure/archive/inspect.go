package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
)

// Inspect はアーカイブの構造を、全体を展開せずに読む。

// Manifest はアーカイブの構造。全体を展開せずに得られる。
type Manifest struct {
	// Roots はアーカイブに含まれるルート（data/world、data/plugins など）。
	Roots []string
	// LevelDir はアーカイブに含まれるワールドのディレクトリ名。
	// 復元先の判定に使う。見つからなければ空。
	LevelDir string
	// LevelDatEntry はワールドの level.dat のエントリ名。
	LevelDatEntry string
	// EntryNames は全エントリの名前。
	EntryNames []string
	// TotalBytes は展開後の合計サイズ。空き容量の事前確認に使う。
	TotalBytes int64
	EntryCount int
}

// Inspect はアーカイブの構造を、全体を展開せずに読む。
// 安全でないエントリがあればエラーを返す。
func Inspect(ctx context.Context, src string) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return Manifest{}, fmt.Errorf("アーカイブを開けません (%s): %w", src, err)
	}
	defer func() { _ = r.Close() }()

	m := Manifest{}
	seenRoots := map[string]bool{}

	for _, f := range r.File {
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return Manifest{}, err
		}
		name, err := safeEntryName(f.Name)
		if err != nil {
			return Manifest{}, err
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}

		m.EntryNames = append(m.EntryNames, name)
		m.TotalBytes += int64(f.UncompressedSize64)
		m.EntryCount++

		if root := rootOf(name); root != "" && !seenRoots[root] {
			seenRoots[root] = true
			m.Roots = append(m.Roots, root)
		}
		if dir, ok := levelDirOf(name); ok {
			m.LevelDir = dir
			m.LevelDatEntry = name
		}
	}
	return m, nil
}

// rootOf は "data/world/level.dat" から "data/world" を、
// "data/spigot.yml" から "data/spigot.yml" を返す。
func rootOf(name string) string {
	rest, ok := strings.CutPrefix(name, allowedRoot)
	if !ok {
		return ""
	}
	first, _, hasMore := strings.Cut(rest, "/")
	if first == "" {
		return ""
	}
	if !hasMore {
		// data/ 直下のファイル
		return allowedRoot + first
	}
	return allowedRoot + first
}

// levelDirOf は "data/<名前>/level.dat" から "<名前>" を返す。
func levelDirOf(name string) (string, bool) {
	if path.Base(name) != "level.dat" {
		return "", false
	}
	rest, ok := strings.CutPrefix(name, allowedRoot)
	if !ok {
		return "", false
	}
	dir, file, hasSlash := strings.Cut(rest, "/")
	if !hasSlash || file != "level.dat" {
		return "", false
	}
	return dir, true
}

// OpenEntry はアーカイブ内の 1 エントリを開く。
//
// 復元の事前確認で level.dat のバージョンを読むために使う。
// zip 全体を展開せずに済むので、30MB のアーカイブでも一瞬で終わる。
func OpenEntry(ctx context.Context, src, entryName string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := safeEntryName(entryName); err != nil {
		return nil, err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, fmt.Errorf("アーカイブを開けません (%s): %w", src, err)
	}

	for _, f := range r.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("エントリを開けません (%s): %w", entryName, err)
		}
		return &entryReader{ReadCloser: rc, archive: r}, nil
	}

	_ = r.Close()
	return nil, fmt.Errorf("エントリが見つかりません: %s", entryName)
}

// entryReader はエントリを閉じるときにアーカイブも閉じる。
type entryReader struct {
	io.ReadCloser
	archive *zip.ReadCloser
}

func (e *entryReader) Close() error {
	err := e.ReadCloser.Close()
	if archiveErr := e.archive.Close(); err == nil {
		err = archiveErr
	}
	return err
}
