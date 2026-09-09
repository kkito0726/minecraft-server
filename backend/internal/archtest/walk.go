package archtest

import (
	"io/fs"
	"path/filepath"
)

// filepathWalkDir はディレクトリを再帰的に辿る。testdata は除く。
func filepathWalkDir(root string, fn func(path string, isDir bool) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return fs.SkipDir
		}
		return fn(path, d.IsDir())
	})
}
