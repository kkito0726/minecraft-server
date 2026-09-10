package rpc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/worldctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/world"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/rpc"
)

// fakeWorldRepo は最小限のワールドリポジトリ。
type fakeWorldRepo struct {
	existing    map[string]bool
	quarantines []world.Quarantine
}

func (f *fakeWorldRepo) List(_ context.Context, active world.Name) ([]world.World, error) {
	var out []world.World
	for name := range f.existing {
		n, err := world.NewName(name)
		if err != nil {
			continue
		}
		out = append(out, world.NewWorld(n, n == active, 1024,
			time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
			shared.UnreadableWorldVersion(), true))
	}
	return out, nil
}

func (f *fakeWorldRepo) Exists(_ context.Context, n world.Name) (bool, error) {
	return f.existing[n.String()], nil
}
func (f *fakeWorldRepo) Copy(_ context.Context, _, dst world.Name, _ port.Progress) error {
	f.existing[dst.String()] = true
	return nil
}
func (f *fakeWorldRepo) Rename(_ context.Context, from, to world.Name) error {
	delete(f.existing, from.String())
	f.existing[to.String()] = true
	return nil
}
func (f *fakeWorldRepo) Quarantine(
	_ context.Context, n world.Name, k world.QuarantineKind,
) (world.Quarantine, error) {
	delete(f.existing, n.String())
	q, err := world.ParseQuarantine(world.NewQuarantineName(n, k, time.Now()), 2048)
	if err != nil {
		return world.Quarantine{}, err
	}
	f.quarantines = append(f.quarantines, q)
	return q, nil
}
func (f *fakeWorldRepo) Restore(context.Context, world.Quarantine, world.Name) error { return nil }
func (f *fakeWorldRepo) Remove(_ context.Context, n world.Name) error {
	delete(f.existing, n.String())
	return nil
}
func (f *fakeWorldRepo) ListQuarantines(context.Context) ([]world.Quarantine, error) {
	return f.quarantines, nil
}
func (f *fakeWorldRepo) RemoveQuarantine(_ context.Context, q world.Quarantine) (int64, error) {
	return q.SizeBytes(), nil
}
func (f *fakeWorldRepo) AvailableBytes(context.Context) (int64, error) { return 1 << 40, nil }

func newWorldClient(t *testing.T, names ...string) (mcadminv1connect.WorldServiceClient, *operations.Manager) {
	t.Helper()

	existing := map[string]bool{}
	for _, n := range names {
		existing[n] = true
	}
	repo := &fakeWorldRepo{existing: existing}

	mgr, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}

	uc, err := worldctl.New(worldctl.Config{
		Runtime: &fakeRuntime{status: server.ContainerStatus{State: server.ContainerMissing}},
		Console: fakeConsole{},
		Worlds:  repo,
		Config: &fakeConfig{snapshot: &fakeSnapshot{values: map[string]string{
			"MC_LEVEL": "world", "MC_VERSION": "26.2",
		}}},
		Levels:     &fakeLevels{version: shared.UnreadableWorldVersion()},
		Operations: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	path, handler := mcadminv1connect.NewWorldServiceHandler(rpc.NewWorldHandler(uc))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return mcadminv1connect.NewWorldServiceClient(srv.Client(), srv.URL), mgr
}

func TestListWorlds(t *testing.T) {
	t.Parallel()

	client, _ := newWorldClient(t, "world", "creative")
	res, err := client.ListWorlds(context.Background(),
		connect.NewRequest(&mcadminv1.ListWorldsRequest{}))
	if err != nil {
		t.Fatalf("ListWorlds に失敗: %v", err)
	}

	if len(res.Msg.GetWorlds()) != 2 {
		t.Errorf("ワールドが %d 件", len(res.Msg.GetWorlds()))
	}
	if res.Msg.GetActiveLevel() != "world" {
		t.Errorf("ActiveLevel が %q", res.Msg.GetActiveLevel())
	}

	for _, w := range res.Msg.GetWorlds() {
		if w.GetName() == "world" && !w.GetActive() {
			t.Error("稼働中の印が付いていない")
		}
		if w.GetLastPlayed() == nil {
			t.Error("LastPlayed が入っていない")
		}
		if !w.GetHasSessionLock() {
			t.Error("HasSessionLock が反映されていない")
		}
	}
}

func TestWorldMutationsReturnOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error)
		want mcadminv1.OperationKind
	}{
		{
			name: "切替",
			call: func(c mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error) {
				r, err := c.SwitchWorld(context.Background(),
					connect.NewRequest(&mcadminv1.SwitchWorldRequest{Name: "creative"}))
				if err != nil {
					return nil, err
				}
				return r.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_WORLD_SWITCH,
		},
		{
			name: "作成",
			call: func(c mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error) {
				r, err := c.CreateWorld(context.Background(),
					connect.NewRequest(&mcadminv1.CreateWorldRequest{Name: "fresh", Seed: "1"}))
				if err != nil {
					return nil, err
				}
				return r.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_WORLD_CREATE,
		},
		{
			name: "複製",
			call: func(c mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error) {
				r, err := c.CloneWorld(context.Background(),
					connect.NewRequest(&mcadminv1.CloneWorldRequest{
						Source: "world", Destination: "copy"}))
				if err != nil {
					return nil, err
				}
				return r.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_WORLD_CLONE,
		},
		{
			name: "改名",
			call: func(c mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error) {
				r, err := c.RenameWorld(context.Background(),
					connect.NewRequest(&mcadminv1.RenameWorldRequest{
						From: "creative", To: "renamed"}))
				if err != nil {
					return nil, err
				}
				return r.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_WORLD_RENAME,
		},
		{
			name: "削除",
			call: func(c mcadminv1connect.WorldServiceClient) (*mcadminv1.Operation, error) {
				r, err := c.DeleteWorld(context.Background(),
					connect.NewRequest(&mcadminv1.DeleteWorldRequest{
						Name: "creative", ConfirmName: "creative", QuarantineInstead: true}))
				if err != nil {
					return nil, err
				}
				return r.Msg.GetOperation(), nil
			},
			want: mcadminv1.OperationKind_OPERATION_KIND_WORLD_DELETE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, _ := newWorldClient(t, "world", "creative")

			op, err := tt.call(client)
			if err != nil {
				t.Fatalf("失敗: %v", err)
			}
			if op.GetKind() != tt.want {
				t.Errorf("Kind が %v。%v のはず", op.GetKind(), tt.want)
			}
		})
	}
}

// 稼働中のワールドの削除は FailedPrecondition。
func TestDeleteActiveWorldReturnsFailedPrecondition(t *testing.T) {
	t.Parallel()

	client, _ := newWorldClient(t, "world")
	_, err := client.DeleteWorld(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteWorldRequest{
			Name: "world", ConfirmName: "world", QuarantineInstead: true}))
	if err == nil {
		t.Fatal("拒否されるはず")
	}
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("コードが %v", got)
	}
}

// 確認名の不一致は InvalidArgument で、理由がそのまま伝わる。
// 打ち間違いに「サーバーのログを確認してください」と返すのは不親切。
func TestDeleteWithWrongConfirmation(t *testing.T) {
	t.Parallel()

	client, _ := newWorldClient(t, "world", "creative")
	_, err := client.DeleteWorld(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteWorldRequest{
			Name: "creative", ConfirmName: "typo", QuarantineInstead: true}))
	if err == nil {
		t.Fatal("拒否されるはず")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("コードが %v。InvalidArgument のはず", got)
	}
	if !strings.Contains(err.Error(), "一致") {
		t.Errorf("理由が伝わらない: %v", err)
	}
}

func TestWorldInvalidNameReturnsInvalidArgument(t *testing.T) {
	t.Parallel()

	client, _ := newWorldClient(t, "world")
	_, err := client.SwitchWorld(context.Background(),
		connect.NewRequest(&mcadminv1.SwitchWorldRequest{Name: "../etc"}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("コードが %v", got)
	}
}

// 退避の一覧と完全削除。
func TestQuarantineFlow(t *testing.T) {
	t.Parallel()

	client, mgr := newWorldClient(t, "world", "creative")

	res, err := client.DeleteWorld(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteWorldRequest{
			Name: "creative", ConfirmName: "creative", QuarantineInstead: true}))
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, mgr, res.Msg.GetOperation().GetId())

	list, err := client.ListWorlds(context.Background(),
		connect.NewRequest(&mcadminv1.ListWorldsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Msg.GetQuarantines()) != 1 {
		t.Fatalf("退避が %d 件", len(list.Msg.GetQuarantines()))
	}

	q := list.Msg.GetQuarantines()[0]
	if q.GetOriginalLevel() != "creative" {
		t.Errorf("OriginalLevel が %q", q.GetOriginalLevel())
	}
	if q.GetFromRestore() {
		t.Error("削除由来なのに FromRestore が真")
	}
	if q.GetQuarantinedAt() == nil {
		t.Error("QuarantinedAt が入っていない")
	}

	purged, err := client.PurgeQuarantine(context.Background(),
		connect.NewRequest(&mcadminv1.PurgeQuarantineRequest{Name: q.GetName()}))
	if err != nil {
		t.Fatal(err)
	}
	if purged.Msg.GetFreedBytes() <= 0 {
		t.Errorf("解放したサイズが %d", purged.Msg.GetFreedBytes())
	}
}

func TestPurgeQuarantineNotFound(t *testing.T) {
	t.Parallel()

	client, _ := newWorldClient(t, "world")
	_, err := client.PurgeQuarantine(context.Background(),
		connect.NewRequest(&mcadminv1.PurgeQuarantineRequest{
			Name: "nope.broken-20260101-000000"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("コードが %v", got)
	}
}
