package dotenv

import (
	"context"
	"fmt"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// Adapter は port.ServerConfig の実装。
type Adapter struct {
	store *FileStore
}

// NewAdapter は .env を扱う Adapter を作る。
func NewAdapter(path string) *Adapter { return &Adapter{store: NewFileStore(path)} }

// Load は .env を読む。
func (a *Adapter) Load(ctx context.Context) (port.ConfigSnapshot, error) {
	snap, err := a.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	return &snapshotAdapter{snapshot: snap}, nil
}

// Save は .env を書き戻す。
func (a *Adapter) Save(ctx context.Context, s port.ConfigSnapshot) error {
	adapted, ok := s.(*snapshotAdapter)
	if !ok {
		return fmt.Errorf("想定しない Snapshot の型です: %T", s)
	}
	return a.store.Save(ctx, adapted.snapshot)
}

// snapshotAdapter は port.ConfigSnapshot の実装。
type snapshotAdapter struct {
	snapshot *Snapshot
}

func (s *snapshotAdapter) Get(key string) (string, bool) { return s.snapshot.File().Get(key) }

func (s *snapshotAdapter) With(key, value string) port.ConfigSnapshot {
	return &snapshotAdapter{snapshot: s.snapshot.WithValue(key, value)}
}

var (
	_ port.ServerConfig   = (*Adapter)(nil)
	_ port.ConfigSnapshot = (*snapshotAdapter)(nil)
)
