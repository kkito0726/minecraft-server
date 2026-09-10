package backupctl_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/backup"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
)

// recorder は呼び出し順を記録する。
//
// 「アーカイブ作成が失敗しても save-on が呼ばれる」のような順序の
// 不変条件は、結果だけを見ても検証できない。
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

type fakeRuntime struct {
	rec    *recorder
	status server.ContainerStatus
}

func (f *fakeRuntime) Up(context.Context, port.LogSink) error   { f.rec.add("Up"); return nil }
func (f *fakeRuntime) Down(context.Context, port.LogSink) error { f.rec.add("Down"); return nil }

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

type fakeConsole struct {
	rec *recorder
}

func (f *fakeConsole) SaveOff(context.Context) error { f.rec.add("SaveOff"); return nil }
func (f *fakeConsole) SaveAll(context.Context) error { f.rec.add("SaveAll"); return nil }
func (f *fakeConsole) SaveOn(context.Context) error  { f.rec.add("SaveOn"); return nil }

func (f *fakeConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return 0, 5, true, nil
}

type fakeSnapshot struct {
	values map[string]string
}

func (f *fakeSnapshot) Get(k string) (string, bool) { v, ok := f.values[k]; return v, ok }

func (f *fakeSnapshot) With(k, v string) port.ConfigSnapshot {
	next := map[string]string{}
	for key, val := range f.values {
		next[key] = val
	}
	next[k] = v
	return &fakeSnapshot{values: next}
}

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
	return &fakeSnapshot{values: copied}, nil
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

// errArchive はアーカイブ作成の失敗を注入するためのエラー。
var errArchive = errors.New("ディスクがいっぱいです")

// fakeStore はアーカイブ保管先の偽物。中身は持たず、名前と大きさだけを持つ。
type fakeStore struct {
	mu  sync.Mutex
	rec *recorder
	// entries は保管中のアーカイブ。ID をキーにする。
	entries map[string]port.StoredBackup
	// createdLevels は Create に渡されたワールド名を記録する。
	createdLevels []string
	createErr     error
	inspectErr    error
	levelDat      string
	info          port.ArchiveInfo
}

func newStore(rec *recorder) *fakeStore {
	return &fakeStore{
		rec:      rec,
		entries:  map[string]port.StoredBackup{},
		levelDat: "level.dat の中身",
		info: port.ArchiveInfo{
			Level:       "world",
			EntryRoots:  []string{"data/world", "data/plugins"},
			TotalBytes:  2048,
			HasLevelDat: true,
		},
	}
}

func (f *fakeStore) Directory() string { return "/tmp/backups" }

func (f *fakeStore) List(context.Context) ([]port.StoredBackup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]port.StoredBackup, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e)
	}
	// 新しい順。実装が並べ替えに依存していないことを確かめるため逆順で返す。
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (f *fakeStore) Create(
	_ context.Context, id backup.ID, level world.Name, progress port.Progress,
) (port.StoredBackup, error) {
	f.rec.add("CreateArchive")

	f.mu.Lock()
	f.createdLevels = append(f.createdLevels, level.String())
	f.mu.Unlock()

	if f.createErr != nil {
		return port.StoredBackup{}, f.createErr
	}
	if progress != nil {
		progress(1024, 1024)
	}

	entry := port.StoredBackup{ID: id, SizeBytes: 1024, CreatedAt: time.Now()}
	f.mu.Lock()
	f.entries[id.String()] = entry
	f.mu.Unlock()
	return entry, nil
}

func (f *fakeStore) Delete(_ context.Context, id backup.ID) (int64, error) {
	f.rec.add("DeleteArchive")

	f.mu.Lock()
	defer f.mu.Unlock()

	entry, ok := f.entries[id.String()]
	if !ok {
		return 0, backup.ErrNotFound
	}
	delete(f.entries, id.String())
	return entry.SizeBytes, nil
}

func (f *fakeStore) Inspect(context.Context, backup.ID) (port.ArchiveInfo, error) {
	if f.inspectErr != nil {
		return port.ArchiveInfo{}, f.inspectErr
	}
	return f.info, nil
}

func (f *fakeStore) OpenLevelDat(context.Context, backup.ID) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.levelDat)), nil
}

// seed は既存のバックアップを流し込む。世代管理の検証に使う。
func (f *fakeStore) seed(t *testing.T, name string, createdAt time.Time) {
	t.Helper()

	id, err := backup.NewID(name)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[name] = port.StoredBackup{ID: id, SizeBytes: 1024, CreatedAt: createdAt}
}

func (f *fakeStore) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, 0, len(f.entries))
	for name := range f.entries {
		out = append(out, name)
	}
	return out
}

func (f *fakeStore) levels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.createdLevels))
	copy(out, f.createdLevels)
	return out
}

type fakeLevels struct {
	version shared.WorldVersion
}

func (f *fakeLevels) ReadWorld(context.Context, world.Name) shared.WorldVersion { return f.version }
func (f *fakeLevels) Read(context.Context, io.Reader) shared.WorldVersion       { return f.version }

// fakeLock は排他の偽物。save-off の記録がいつ切り替わったかを覚える。
//
// MarkSaveDisabled は「バックアップの途中で落ちたことを次の起動で
// 検出する」ための唯一の手段なので、呼ばれていることを検証する。
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

// saveDisabledHistory は SaveDisabled の記録の変遷を返す。
func (f *fakeLock) saveDisabledHistory() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]bool, len(f.saveFlag))
	copy(out, f.saveFlag)
	return out
}

type fakeHandle struct{ lock *fakeLock }

func (h *fakeHandle) Release() error { return h.lock.ForceRemove() }

// harness はユースケースと偽物一式。
type harness struct {
	uc      *backupctl.UseCase
	ops     *operations.Manager
	rec     *recorder
	store   *fakeStore
	config  *fakeConfig
	lock    *fakeLock
	runtime *fakeRuntime
}

func newHarness(t *testing.T, running bool, values map[string]string) *harness {
	t.Helper()

	rec := &recorder{}
	state := server.ContainerMissing
	if running {
		state = server.ContainerRunning
	}

	if values == nil {
		values = map[string]string{"MC_LEVEL": "world", "MC_VERSION": "26.2"}
	}

	runtime := &fakeRuntime{rec: rec, status: server.ContainerStatus{State: state}}
	store := newStore(rec)
	config := newConfig(rec, values)
	lock := &fakeLock{}

	mgr, err := operations.NewManager(operations.Config{Lock: lock})
	if err != nil {
		t.Fatal(err)
	}

	uc, err := backupctl.New(backupctl.Config{
		Runtime:    runtime,
		Console:    &fakeConsole{rec: rec},
		Store:      store,
		Config:     config,
		Levels:     &fakeLevels{version: shared.UnreadableWorldVersion()},
		Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &harness{
		uc: uc, ops: mgr, rec: rec, store: store,
		config: config, lock: lock, runtime: runtime,
	}
}

func (h *harness) wait(t *testing.T, id operation.ID) operation.Snapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := h.ops.Get(id); ok && snap.State.IsTerminal() {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
	return operation.Snapshot{}
}

func (h *harness) calls() string { return strings.Join(h.rec.list(), ",") }
