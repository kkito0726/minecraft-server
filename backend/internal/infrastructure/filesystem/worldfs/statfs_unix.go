//go:build unix

package worldfs

import (
	"fmt"
	"syscall"
)

// availableBytes はファイルシステムの空き容量を返す。
//
// 展開や複製を始める前に確認する。途中で容量が尽きると、
// 中途半端なディレクトリが残って復旧が面倒になる。
func availableBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("空き容量を取得できません (%s): %w", path, err)
	}
	//nolint:gosec // Bavail と Bsize は非負。オーバーフローは現実的でない
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
