//go:build unix

package system

import (
	"fmt"
	"syscall"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// readStorage は path が載っているファイルシステムの使用状況を返す。
//
// 使うのは Bavail で、Bfree ではない。ext4 は既定で 5% を root 用に
// 予約しており、Bfree はそれを含む。df が「使用可能」に出すのは Bavail
// なので、こちらに合わせないと画面と df の数字が食い違う。
//
// 使用量も同じ理由で total-available ではなく Blocks-Bfree で出す。
// 予約分を「誰かが使っている量」に数えないため。
func readStorage(path string) (port.StorageUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return port.StorageUsage{}, fmt.Errorf("容量を取得できません (%s): %w", path, err)
	}

	//nolint:gosec // Blocks / Bavail / Bfree / Bsize は非負
	blockSize := int64(stat.Bsize)
	//nolint:gosec // 同上
	total := int64(stat.Blocks) * blockSize
	//nolint:gosec // 同上
	available := int64(stat.Bavail) * blockSize
	//nolint:gosec // 同上
	used := (int64(stat.Blocks) - int64(stat.Bfree)) * blockSize

	if total <= 0 {
		return port.StorageUsage{}, fmt.Errorf("容量が 0 です (%s)", path)
	}

	return port.StorageUsage{
		Available:      true,
		Path:           path,
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsedPercent:    float64(used) / float64(total) * 100,
	}, nil
}
