//go:build !unix

package system

import (
	"errors"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

func readStorage(path string) (port.StorageUsage, error) {
	return port.StorageUsage{}, errors.New("この OS では容量を取得できません")
}
