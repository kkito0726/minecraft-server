// Package rpccontract は proto スキーマのうち、設計が依存している性質を固定する。
//
// ここにあるのは「生成コードのテスト」ではなく「契約の回帰テスト」。
// たとえば WatchOperation を単項 RPC に変えると、再読み込み後に進捗を
// 再送する仕組み（REQ-127）が成立しなくなるが、コンパイルは通ってしまう。
// そういう静かな破壊を検出する。
package rpccontract

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
)

// 進捗の配信は server-streaming でなければならない。
// 単項に変えるとリロード後の再送（REQ-127）と再接続（REQ-128）が成立しない。
func TestWatchOperationIsServerStreaming(t *testing.T) {
	t.Parallel()

	method := lookupMethod(t, "mcadmin.v1.OperationService", "WatchOperation")

	if !method.IsStreamingServer() {
		t.Error("WatchOperation が server-streaming ではない。リロード後の進捗再送が成立しなくなる")
	}
	if method.IsStreamingClient() {
		t.Error("WatchOperation が client-streaming になっている")
	}
}

// 再接続の位置指定に使う from_seq が消えると、再接続のたびに
// 最初から再送するか、逆に取りこぼすかのどちらかになる。
func TestWatchOperationRequestHasFromSeq(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.WatchOperationRequest{}).ProtoReflect().Descriptor()
	field := msg.Fields().ByName("from_seq")
	if field == nil {
		t.Fatal("WatchOperationRequest に from_seq がない")
	}
	if got := field.Kind(); got != protoreflect.Int64Kind {
		t.Errorf("from_seq の型が %v。単調増加する位置なので int64 であること", got)
	}
}

// 各イベントは完全な状態（snapshot）を同梱する。
// これが無いと、イベントを 1 つ取りこぼしただけで画面の状態がずれる。
func TestWatchOperationResponseCarriesSnapshot(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.WatchOperationResponse{}).ProtoReflect().Descriptor()

	seq := msg.Fields().ByName("seq")
	if seq == nil || seq.Kind() != protoreflect.Int64Kind {
		t.Error("WatchOperationResponse に int64 の seq がない")
	}

	snapshot := msg.Fields().ByName("snapshot")
	if snapshot == nil {
		t.Fatal("WatchOperationResponse に snapshot がない。1 イベントで状態を復元できなくなる")
	}
	if got := snapshot.Message().FullName(); got != "mcadmin.v1.Operation" {
		t.Errorf("snapshot の型が %v。Operation であること", got)
	}
}

// バージョン判定のキーは整数の data_version。
// 表示文字列 name で比較すると、同じ "26.2" でも DataVersion が違う
// ケース（スナップショット間の差など）を取り違える（REQ-412）。
func TestWorldVersionHasIntegerDataVersion(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.WorldVersion{}).ProtoReflect().Descriptor()

	dataVersion := msg.Fields().ByName("data_version")
	if dataVersion == nil {
		t.Fatal("WorldVersion に data_version がない")
	}
	if got := dataVersion.Kind(); got != protoreflect.Int32Kind {
		t.Errorf("data_version の型が %v。level.dat の DataVersion は整数なので int32 であること", got)
	}

	// readable が無いと「読めなかった」と「バージョン 0」を区別できない（EDGE-001）
	if msg.Fields().ByName("readable") == nil {
		t.Error("WorldVersion に readable がない。読めなかった場合を表現できない")
	}
}

// v1 に中断（Cancel）を設けない（REQ-417）。
// 復元の途中で止めると「退避済みかつ展開途中」という最も復旧が困難な状態になる。
func TestNoCancelOperation(t *testing.T) {
	t.Parallel()

	svc := lookupService(t, "mcadmin.v1.OperationService")
	for i := range svc.Methods().Len() {
		name := string(svc.Methods().Get(i).Name())
		if name == "CancelOperation" {
			t.Error("CancelOperation が追加されている。v1 では意図的に設けない（REQ-417）")
		}
	}
}

// 復元はバージョン警告の承諾と、対象名の完全一致入力の両方を要求する。
func TestRestoreRequiresConfirmation(t *testing.T) {
	t.Parallel()

	msg := (&mcadminv1.RestoreBackupRequest{}).ProtoReflect().Descriptor()
	for _, name := range []string{"acknowledge_version_warning", "confirm_level_name"} {
		if msg.Fields().ByName(protoreflect.Name(name)) == nil {
			t.Errorf("RestoreBackupRequest に %s がない。危険な操作の確認が外れている", name)
		}
	}
}

// 変更を伴う RPC はすべて Operation を返す。同期的に完了したことにすると、
// 進捗の購読も排他ロックの表示もできなくなる。
func TestMutatingRPCsReturnOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		service string
		methods []string
	}{
		{
			service: "mcadmin.v1.ServerService",
			methods: []string{"StartServer", "StopServer", "RestartServer"},
		},
		{
			service: "mcadmin.v1.WorldService",
			methods: []string{"SwitchWorld", "CreateWorld", "CloneWorld", "RenameWorld", "DeleteWorld"},
		},
		{
			service: "mcadmin.v1.BackupService",
			methods: []string{"CreateBackup", "RestoreBackup"},
		},
	}

	for _, tt := range tests {
		for _, m := range tt.methods {
			t.Run(tt.service+"/"+m, func(t *testing.T) {
				t.Parallel()
				method := lookupMethod(t, protoreflect.FullName(tt.service), protoreflect.Name(m))
				field := method.Output().Fields().ByName("operation")
				if field == nil {
					t.Fatalf("%s の応答に operation がない", m)
				}
				if got := field.Message().FullName(); got != "mcadmin.v1.Operation" {
					t.Errorf("operation の型が %v", got)
				}
			})
		}
	}
}

// PreflightRestore は副作用を持たない読み取り専用の RPC なので、
// Operation ではなく判定結果そのものを返す（REQ-008）。
func TestPreflightReturnsVerdictNotOperation(t *testing.T) {
	t.Parallel()

	method := lookupMethod(t, "mcadmin.v1.BackupService", "PreflightRestore")
	out := method.Output()

	if out.Fields().ByName("operation") != nil {
		t.Error("PreflightRestore が Operation を返している。副作用を持たない RPC のはず")
	}
	for _, name := range []string{"verdict", "requires_confirmation", "level_name_mismatch"} {
		if out.Fields().ByName(protoreflect.Name(name)) == nil {
			t.Errorf("PreflightRestoreResponse に %s がない", name)
		}
	}
}

func lookupService(t *testing.T, name protoreflect.FullName) protoreflect.ServiceDescriptor {
	t.Helper()

	for _, file := range []protoreflect.FileDescriptor{
		mcadminv1.File_mcadmin_v1_operation_proto,
		mcadminv1.File_mcadmin_v1_server_proto,
		mcadminv1.File_mcadmin_v1_world_proto,
		mcadminv1.File_mcadmin_v1_backup_proto,
	} {
		services := file.Services()
		for i := range services.Len() {
			if svc := services.Get(i); svc.FullName() == name {
				return svc
			}
		}
	}
	t.Fatalf("サービス %s が見つからない", name)
	return nil
}

func lookupMethod(t *testing.T, service protoreflect.FullName, method protoreflect.Name) protoreflect.MethodDescriptor {
	t.Helper()

	m := lookupService(t, service).Methods().ByName(method)
	if m == nil {
		t.Fatalf("%s に %s がない", service, method)
	}
	return m
}
