package worldctl_test

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// recorder は呼び出し順を記録する。
//
// 「退避が展開より先」「失敗しても save-on が呼ばれる」といった
// 順序の不変条件は、結果だけを見ても検証できない。
type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

// fakeRuntime は docker compose の偽物。
type fakeRuntime struct {
	rec    *recorder
	status server.ContainerStatus
	upErr  error
}

func (f *fakeRuntime) Up(context.Context, port.LogSink) error {
	f.rec.add("Up")
	return f.upErr
}

func (f *fakeRuntime) Down(context.Context, port.LogSink) error {
	f.rec.add("Down")
	return nil
}

func (f *fakeRuntime) Status(context.Context) (server.ContainerStatus, error) {
	return f.status, nil
}

func (f *fakeRuntime) WaitStopped(context.Context, time.Duration) error {
	f.rec.add("WaitStopped")
	return nil
}

func (f *fakeRuntime) WaitReady(context.Context, time.Duration) error {
	f.rec.add("WaitReady")
	return nil
}

// fakeConsole は RCON の偽物。
type fakeConsole struct {
	rec        *recorder
	saveAllErr error
}

func (f *fakeConsole) SaveOff(context.Context) error { f.rec.add("SaveOff"); return nil }
func (f *fakeConsole) SaveOn(context.Context) error  { f.rec.add("SaveOn"); return nil }

func (f *fakeConsole) SaveAll(context.Context) error {
	f.rec.add("SaveAll")
	return f.saveAllErr
}

func (f *fakeConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return 0, 5, true, nil
}

// fakeSnapshot は .env の 1 断面。
type fakeSnapshot struct {
	values map[string]string
	store  *fakeConfig
}

func (f *fakeSnapshot) Get(k string) (string, bool) { v, ok := f.values[k]; return v, ok }

func (f *fakeSnapshot) With(k, v string) port.ConfigSnapshot {
	next := map[string]string{}
	for key, val := range f.values {
		next[key] = val
	}
	next[k] = v
	return &fakeSnapshot{values: next, store: f.store}
}

// fakeConfig は .env の偽物。保存された内容を検査できる。
type fakeConfig struct {
	mu     sync.Mutex
	rec    *recorder
	values map[string]string
}

func newConfig(rec *recorder, values map[string]string) *fakeConfig {
	return &fakeConfig{rec: rec, values: values}
}

func (f *fakeConfig) Load(context.Context) (port.ConfigSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	copied := map[string]string{}
	for k, v := range f.values {
		copied[k] = v
	}
	return &fakeSnapshot{values: copied, store: f}, nil
}

func (f *fakeConfig) Save(_ context.Context, s port.ConfigSnapshot) error {
	f.rec.add("SaveConfig")

	snap, ok := s.(*fakeSnapshot)
	if !ok {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = snap.values
	return nil
}

func (f *fakeConfig) get(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[key]
}

// fakeWorlds はワールドディレクトリの偽物。
type fakeWorlds struct {
	mu          sync.Mutex
	rec         *recorder
	existing    map[string]bool
	quarantines []world.Quarantine
	available   int64
	copyErr     error
}

func newWorlds(rec *recorder, names ...string) *fakeWorlds {
	existing := map[string]bool{}
	for _, n := range names {
		existing[n] = true
	}
	return &fakeWorlds{rec: rec, existing: existing, available: 1 << 40}
}

func (f *fakeWorlds) List(context.Context, world.Name) ([]world.World, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []world.World
	for name := range f.existing {
		n, err := world.NewName(name)
		if err != nil {
			continue
		}
		out = append(out, world.NewWorld(n, false, 1024, time.Time{},
			shared.UnreadableWorldVersion(), false))
	}
	return out, nil
}

func (f *fakeWorlds) Exists(_ context.Context, name world.Name) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing[name.String()], nil
}

func (f *fakeWorlds) Copy(_ context.Context, src, dst world.Name, _ port.Progress) error {
	f.rec.add("Copy")
	if f.copyErr != nil {
		return f.copyErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existing[dst.String()] = true
	_ = src
	return nil
}

func (f *fakeWorlds) Rename(_ context.Context, from, to world.Name) error {
	f.rec.add("Rename")
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.existing, from.String())
	f.existing[to.String()] = true
	return nil
}

func (f *fakeWorlds) Quarantine(
	_ context.Context, name world.Name, kind world.QuarantineKind,
) (world.Quarantine, error) {
	f.rec.add("Quarantine")
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.existing, name.String())

	dirName := world.NewQuarantineName(name, kind, time.Now())
	q, err := world.ParseQuarantine(dirName, 1024)
	if err != nil {
		return world.Quarantine{}, err
	}
	f.quarantines = append(f.quarantines, q)
	return q, nil
}

func (f *fakeWorlds) Restore(_ context.Context, _ world.Quarantine, to world.Name) error {
	f.rec.add("RestoreQuarantine")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existing[to.String()] = true
	return nil
}

func (f *fakeWorlds) Remove(_ context.Context, name world.Name) error {
	f.rec.add("Remove")
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.existing, name.String())
	return nil
}

func (f *fakeWorlds) ListQuarantines(context.Context) ([]world.Quarantine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]world.Quarantine, len(f.quarantines))
	copy(out, f.quarantines)
	return out, nil
}

func (f *fakeWorlds) RemoveQuarantine(_ context.Context, q world.Quarantine) (int64, error) {
	f.rec.add("RemoveQuarantine")
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, existing := range f.quarantines {
		if existing.DirName() == q.DirName() {
			f.quarantines = append(f.quarantines[:i], f.quarantines[i+1:]...)
			break
		}
	}
	return q.SizeBytes(), nil
}

func (f *fakeWorlds) AvailableBytes(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available, nil
}

func (f *fakeWorlds) has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing[name]
}

// fakeLevels は level.dat の読み取りの偽物。
type fakeLevels struct{ version shared.WorldVersion }

func (f *fakeLevels) ReadWorld(context.Context, world.Name) shared.WorldVersion { return f.version }
func (f *fakeLevels) Read(context.Context, io.Reader) shared.WorldVersion       { return f.version }

// fakeLock は排他の偽物。save-off の記録がいつ切り替わったかを覚える。
//
// MarkSaveDisabled は「複製の途中で落ちたことを次の起動で検出する」
// ための唯一の手段なので、呼ばれていることを検証できるようにする。
type fakeLock struct {
	mu       sync.Mutex
	meta     port.LockMeta
	held     bool
	saveFlag []bool
}

func (f *fakeLock) Acquire(meta port.LockMeta) (port.LockHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.held {
		return nil, port.ErrLocked
	}
	f.held = true
	f.meta = meta
	f.saveFlag = append(f.saveFlag, meta.SaveDisabled)
	return &fakeHandle{lock: f}, nil
}

func (f *fakeLock) Update(meta port.LockMeta) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.meta = meta
	f.saveFlag = append(f.saveFlag, meta.SaveDisabled)
	return nil
}

func (f *fakeLock) Inspect() (port.LockMeta, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.meta, f.held, nil
}

func (f *fakeLock) IsStale(port.LockMeta) bool { return false }

func (f *fakeLock) ForceRemove() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = false
	return nil
}

func (f *fakeLock) saveDisabledHistory() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]bool, len(f.saveFlag))
	copy(out, f.saveFlag)
	return out
}

type fakeHandle struct{ lock *fakeLock }

func (h *fakeHandle) Release() error { return h.lock.ForceRemove() }
