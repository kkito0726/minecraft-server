package rpc_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/serverctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/rpc"
)

// --- 偽の依存 ---

type fakeRuntime struct {
	status server.ContainerStatus
	block  chan struct{}
}

func (f *fakeRuntime) Up(context.Context, port.LogSink) error {
	if f.block != nil {
		<-f.block
	}
	return nil
}
func (f *fakeRuntime) Down(context.Context, port.LogSink) error { return nil }
func (f *fakeRuntime) Status(context.Context) (server.ContainerStatus, error) {
	return f.status, nil
}
func (f *fakeRuntime) WaitStopped(context.Context, time.Duration) error { return nil }
func (f *fakeRuntime) WaitReady(context.Context, time.Duration) error   { return nil }

type fakeConsole struct{}

func (fakeConsole) SaveOff(context.Context) error { return nil }
func (fakeConsole) SaveAll(context.Context) error { return nil }
func (fakeConsole) SaveOn(context.Context) error  { return nil }
func (fakeConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return 3, 5, true, nil
}

type fakeSnapshot struct{ values map[string]string }

func (f *fakeSnapshot) Get(k string) (string, bool) { v, ok := f.values[k]; return v, ok }
func (f *fakeSnapshot) With(k, v string) port.ConfigSnapshot {
	next := map[string]string{}
	for key, val := range f.values {
		next[key] = val
	}
	next[k] = v
	return &fakeSnapshot{values: next}
}

type fakeConfig struct{ snapshot *fakeSnapshot }

func (f *fakeConfig) Load(context.Context) (port.ConfigSnapshot, error) { return f.snapshot, nil }
func (f *fakeConfig) Save(context.Context, port.ConfigSnapshot) error   { return nil }

type fakeLevels struct{ version shared.WorldVersion }

func (f *fakeLevels) ReadWorld(context.Context, world.Name) shared.WorldVersion { return f.version }
func (f *fakeLevels) Read(context.Context, io.Reader) shared.WorldVersion       { return f.version }

// --- テストサーバー ---

type harness struct {
	serverClient mcadminv1connect.ServerServiceClient
	opClient     mcadminv1connect.OperationServiceClient
	ops          *operations.Manager
	runtime      *fakeRuntime
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	rt := &fakeRuntime{status: server.ContainerStatus{
		State:     server.ContainerRunning,
		Healthy:   true,
		StartedAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		Image:     "itzg/minecraft-server:latest",
	}}

	dv, err := shared.NewDataVersion(4903)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &fakeConfig{snapshot: &fakeSnapshot{values: map[string]string{
		"MC_VERSION": "26.2", "MC_LEVEL": "world",
	}}}

	ops, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := serverctl.NewStatusUseCase(serverctl.StatusConfig{
		Runtime: rt, Console: fakeConsole{}, Config: cfg,
		Levels: &fakeLevels{version: shared.NewWorldVersion("26.2", dv, false, "world")},
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := serverctl.NewLifecycleUseCase(serverctl.LifecycleConfig{
		Runtime: rt, Console: fakeConsole{}, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	sp, sh := mcadminv1connect.NewServerServiceHandler(
		rpc.NewServerHandler(status, lifecycle, ops, nil))
	mux.Handle(sp, sh)
	op, oh := mcadminv1connect.NewOperationServiceHandler(rpc.NewOperationHandler(ops))
	mux.Handle(op, oh)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &harness{
		serverClient: mcadminv1connect.NewServerServiceClient(srv.Client(), srv.URL),
		opClient:     mcadminv1connect.NewOperationServiceClient(srv.Client(), srv.URL),
		ops:          ops,
		runtime:      rt,
	}
}

// --- テスト ---

func TestGetStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.GetStatus(context.Background(),
		connect.NewRequest(&mcadminv1.GetStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStatus に失敗: %v", err)
	}

	msg := res.Msg
	if msg.GetContainerState() != mcadminv1.ContainerState_CONTAINER_STATE_RUNNING {
		t.Errorf("ContainerState が %v", msg.GetContainerState())
	}
	if !msg.GetHealthy() {
		t.Error("Healthy が偽")
	}
	if msg.GetActiveLevel() != "world" || msg.GetConfiguredVersion() != "26.2" {
		t.Errorf("Level=%q Version=%q", msg.GetActiveLevel(), msg.GetConfiguredVersion())
	}
	if msg.GetOnlinePlayers() != 3 || msg.GetMaxPlayers() != 5 {
		t.Errorf("人数が %d/%d", msg.GetOnlinePlayers(), msg.GetMaxPlayers())
	}
	if !msg.GetActiveWorldVersion().GetReadable() {
		t.Error("ワールドのバージョンが読めていない")
	}
}

// 実行中の操作が状態に含まれる。画面が排他表示に切り替わるため。
func TestGetStatusIncludesActiveOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.runtime.block = make(chan struct{})

	if _, err := h.serverClient.StartServer(context.Background(),
		connect.NewRequest(&mcadminv1.StartServerRequest{})); err != nil {
		t.Fatal(err)
	}
	waitActive(t, h.ops)

	res, err := h.serverClient.GetStatus(context.Background(),
		connect.NewRequest(&mcadminv1.GetStatusRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetActiveOperation() == nil {
		t.Error("実行中の操作が含まれていない")
	}

	close(h.runtime.block)
}

// 実行中は次の操作を拒否し、FailedPrecondition を返す。
func TestStartServerWhileBusy(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.runtime.block = make(chan struct{})

	if _, err := h.serverClient.StartServer(context.Background(),
		connect.NewRequest(&mcadminv1.StartServerRequest{})); err != nil {
		t.Fatal(err)
	}
	waitActive(t, h.ops)

	_, err := h.serverClient.RestartServer(context.Background(),
		connect.NewRequest(&mcadminv1.RestartServerRequest{}))
	if err == nil {
		t.Fatal("拒否されるはず")
	}
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("コードが %v。FailedPrecondition のはず", got)
	}

	close(h.runtime.block)
}

func TestServerLifecycleReturnsOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(*harness) (*mcadminv1.Operation, error)
		want mcadminv1.OperationKind
	}{
		{
			name: "起動",
			call: func(h *harness) (*mcadminv1.Operation, error) {
				res, err := h.serverClient.StartServer(context.Background(),
					connect.NewRequest(&mcadminv1.StartServerRequest{}))
				if err != nil {
					return nil, err
				}
				return res.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_SERVER_START,
		},
		{
			name: "停止",
			call: func(h *harness) (*mcadminv1.Operation, error) {
				res, err := h.serverClient.StopServer(context.Background(),
					connect.NewRequest(&mcadminv1.StopServerRequest{}))
				if err != nil {
					return nil, err
				}
				return res.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_SERVER_STOP,
		},
		{
			name: "再起動",
			call: func(h *harness) (*mcadminv1.Operation, error) {
				res, err := h.serverClient.RestartServer(context.Background(),
					connect.NewRequest(&mcadminv1.RestartServerRequest{}))
				if err != nil {
					return nil, err
				}
				return res.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_SERVER_RESTART,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)

			op, err := tt.call(h)
			if err != nil {
				t.Fatalf("失敗: %v", err)
			}
			if op == nil {
				t.Fatal("Operation が返っていない")
			}
			if op.GetKind() != tt.want {
				t.Errorf("Kind が %v。%v のはず", op.GetKind(), tt.want)
			}
			if op.GetId() == "" {
				t.Error("識別子が空")
			}
		})
	}
}

func TestGetActiveOperationWhenIdle(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.opClient.GetActiveOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetActiveOperationRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetPresent() {
		t.Error("何も実行していないのに Present が真")
	}
}

func TestGetOperationNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	_, err := h.opClient.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: "nonexistent"}))
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("コードが %v", got)
	}
}

func TestGetOperationInvalidID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	_, err := h.opClient.GetOperation(context.Background(),
		connect.NewRequest(&mcadminv1.GetOperationRequest{OperationId: ""}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("コードが %v。InvalidArgument のはず", got)
	}
}

// 進捗ストリームが最初から再送される。
// リロード直後の画面がスピナーだけにならないための土台。
func TestWatchOperationReplays(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.StartServer(context.Background(),
		connect.NewRequest(&mcadminv1.StartServerRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	opID := res.Msg.GetOperation().GetId()
	waitFinished(t, h.ops, opID)

	stream, err := h.opClient.WatchOperation(context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: opID, FromSeq: 0}))
	if err != nil {
		t.Fatal(err)
	}

	var events []*mcadminv1.WatchOperationResponse
	for stream.Receive() {
		events = append(events, stream.Msg())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("ストリームが失敗: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("イベントを受け取れていない")
	}

	// seq は 1 始まりで単調増加
	for i, e := range events {
		if want := int64(i + 1); e.GetSeq() != want {
			t.Errorf("%d 番目の Seq が %d。%d のはず", i, e.GetSeq(), want)
		}
		if e.GetSnapshot() == nil {
			t.Errorf("%d 番目に Snapshot が無い", i)
		}
	}
	// 最後は終端状態
	last := events[len(events)-1].GetSnapshot().GetState()
	if last != mcadminv1.OperationState_OPERATION_STATE_SUCCEEDED {
		t.Errorf("最後の State が %v", last)
	}
}

// fromSeq を指定すると、その位置以降だけが再送される。
func TestWatchOperationFromSeq(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.StartServer(context.Background(),
		connect.NewRequest(&mcadminv1.StartServerRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	opID := res.Msg.GetOperation().GetId()
	waitFinished(t, h.ops, opID)

	stream, err := h.opClient.WatchOperation(context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: opID, FromSeq: 2}))
	if err != nil {
		t.Fatal(err)
	}

	var first int64
	n := 0
	for stream.Receive() {
		if n == 0 {
			first = stream.Msg().GetSeq()
		}
		n++
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if first != 2 {
		t.Errorf("最初の Seq が %d。2 のはず", first)
	}
}

func TestWatchOperationNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stream, err := h.opClient.WatchOperation(context.Background(),
		connect.NewRequest(&mcadminv1.WatchOperationRequest{OperationId: "nope"}))
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err == nil {
		t.Fatal("エラーになるはず")
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("コードが %v", got)
	}
}

func TestListOperations(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.serverClient.StartServer(context.Background(),
		connect.NewRequest(&mcadminv1.StartServerRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, h.ops, res.Msg.GetOperation().GetId())

	list, err := h.opClient.ListOperations(context.Background(),
		connect.NewRequest(&mcadminv1.ListOperationsRequest{Limit: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Msg.GetOperations()) != 1 {
		t.Errorf("履歴が %d 件", len(list.Msg.GetOperations()))
	}
}

// --- ヘルパー ---

func waitActive(t *testing.T, m *operations.Manager) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := m.Active(); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が開始しない")
}

func waitFinished(t *testing.T, m *operations.Manager, id string) {
	t.Helper()

	opID, err := operation.NewID(id)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := m.Get(opID); ok && snap.State.IsTerminal() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
}

var _ = errors.New
