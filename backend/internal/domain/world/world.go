package world

import (
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
)

// World は data/ 配下のワールド 1 つ。
type World struct {
	name        Name
	active      bool
	sizeBytes   int64
	lastPlayed  time.Time
	version     shared.WorldVersion
	sessionLock bool
}

// NewWorld はワールドを作る。
func NewWorld(
	name Name,
	active bool,
	sizeBytes int64,
	lastPlayed time.Time,
	version shared.WorldVersion,
	sessionLock bool,
) World {
	return World{
		name:        name,
		active:      active,
		sizeBytes:   sizeBytes,
		lastPlayed:  lastPlayed,
		version:     version,
		sessionLock: sessionLock,
	}
}

// Name はワールド名を返す。
func (w World) Name() Name { return w.name }

// IsActive は .env の MC_LEVEL と一致しているかを返す。
func (w World) IsActive() bool { return w.active }

// SizeBytes はディレクトリのサイズを返す。
func (w World) SizeBytes() int64 { return w.sizeBytes }

// LastPlayed は level.dat の LastPlayed を返す。
func (w World) LastPlayed() time.Time { return w.lastPlayed }

// Version は level.dat から読んだバージョンを返す。
func (w World) Version() shared.WorldVersion { return w.version }

// HasSessionLock は session.lock が残っているかを返す。
// 停止時に消えないため、これ自体は異常ではない。
func (w World) HasSessionLock() bool { return w.sessionLock }

// CanDelete は削除してよいかを返す。
//
// 稼働中のワールドを消すと復旧できない。先に切り替えてもらう。
func (w World) CanDelete() error {
	if w.active {
		return ErrActiveWorld
	}
	return nil
}
