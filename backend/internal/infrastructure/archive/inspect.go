package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
)

// Inspect はアーカイブの構造を、全体を展開せずに読む。

// Manifest はアーカイブの構造。全体を展開せずに得られる。
type Manifest struct {
	// Layout はエントリの形。展開の基点と置き場所の検証がこれで決まる。
	Layout Layout
	// Roots はアーカイブに含まれるルート（data/world、world、plugins など）。
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

	/*
		安全の検証（展開先の外へ出る経路）と、置き場所の検証（形に
		合っているか）を分けて 2 段で行う。

		形は level.dat の位置から決まるので、全エントリを見終わるまで
		分からない。先に置き場所まで検証しようとすると、形が確定する
		前に判断することになる。

		zip.Reader は中央ディレクトリを既に読み込んでいるので、
		2 周してもファイルは読み直さない。
	*/
	names, sizes, err := collectEntries(r)
	if err != nil {
		return Manifest{}, err
	}

	m := Manifest{Layout: detectLayout(names)}
	seenRoots := map[string]bool{}

	for i, name := range names {
		// 置き場所は形が決まってから検証する。
		if err := checkPlacement(name, m.Layout); err != nil {
			return Manifest{}, err
		}

		m.EntryNames = append(m.EntryNames, name)
		m.TotalBytes += sizes[i]
		m.EntryCount++

		if root := rootOf(name, m.Layout); root != "" && !seenRoots[root] {
			seenRoots[root] = true
			m.Roots = append(m.Roots, root)
		}
		if dir, ok := levelDirOf(name, m.Layout); ok {
			m.LevelDir = dir
			m.LevelDatEntry = name
		}
	}
	return m, nil
}

// collectEntries は安全の検証だけを通して、ファイルのエントリ名と
// 展開後の大きさを集める。ディレクトリのエントリは落とす。
func collectEntries(r *zip.ReadCloser) (names []string, sizes []int64, err error) {
	for _, f := range r.File {
		if err := checkMode(f.Name, f.Mode()); err != nil {
			return nil, nil, err
		}
		cleaned, _, err := normalizeEntryName(f.Name)
		if err != nil {
			return nil, nil, err
		}
		if isDirEntry(f.Name) {
			continue
		}
		names = append(names, cleaned)
		sizes = append(sizes, int64(f.UncompressedSize64))
	}
	return names, sizes, nil
}

// OpenEntry はアーカイブ内の 1 エントリを開く。
//
// 復元の事前確認で level.dat のバージョンを読むために使う。
// zip 全体を展開せずに済むので、30MB のアーカイブでも一瞬で終わる。
func OpenEntry(ctx context.Context, src, entryName string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 置き場所は問わない。Inspect が返した名前をそのまま受けるため、
	// 形ごとに判定を持ち込む理由がない。読むだけで何も書かない。
	if _, _, err := normalizeEntryName(entryName); err != nil {
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
