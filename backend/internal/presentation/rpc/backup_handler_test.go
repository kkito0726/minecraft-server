package rpc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/backupctl"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/shared"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/filesystem/worldfs"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/backupfs"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/rpc"
)

// backupHarness は BackupService を実物のファイルシステム上で動かす。
//
// docker と RCON だけを偽物にし、zip の作成は本物を使う。
// 「実際に取得したものが展開できる」ことまで含めて確かめるため。
type backupHarness struct {
	client    mcadminv1connect.BackupServiceClient
	ops       *operations.Manager
	root      string
	backupDir string
}

func newBackupHarness(t *testing.T) *backupHarness {
	t.Helper()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "data", "world", "level.dat"), "level")
	writeTestFile(t, filepath.Join(root, "data", "plugins", "Essentials.jar"), "plugin")
	writeTestFile(t, filepath.Join(root, "data", "server.properties"), "rcon.password=ひみつ")

	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")
	store, err := backupfs.New(backupfs.Config{ProjectDir: root, BackupDir: backupDir})
	if err != nil {
		t.Fatal(err)
	}

	ops, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}

	dv, err := shared.NewDataVersion(4903)
	if err != nil {
		t.Fatal(err)
	}

	levels := &fakeLevels{version: shared.NewWorldVersion("26.2", dv, false, "world")}
	// ワールドの退避は実物のファイルシステム上で行う。
	// 「退避してから展開」が本当に mv になっているかは偽物では確かめられない。
	worlds, err := worldfs.New(worldfs.Config{
		DataDir:  dataDir,
		Versions: fileVersions{version: levels.version},
	})
	if err != nil {
		t.Fatal(err)
	}

	uc, err := backupctl.New(backupctl.Config{
		Runtime: &fakeRuntime{status: server.ContainerStatus{State: server.ContainerRunning}},
		Console: fakeConsole{},
		Store:   store,
		Config: &fakeConfig{snapshot: &fakeSnapshot{values: map[string]string{
			"MC_VERSION": "26.2", "MC_LEVEL": "world", "ADMIN_BACKUP_KEEP": "10",
		}}},
		Worlds:     worlds,
		Levels:     levels,
		Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	path, handler := mcadminv1connect.NewBackupServiceHandler(rpc.NewBackupHandler(uc))
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &backupHarness{
		client:    mcadminv1connect.NewBackupServiceClient(srv.Client(), srv.URL),
		ops:       ops,
		root:      root,
		backupDir: backupDir,
	}
}

// fileVersions は worldfs 用のバージョン読み取りの偽物。
// この試験の関心は退避と展開の順序であって NBT の解釈ではない。
type fileVersions struct{ version shared.WorldVersion }

func (f fileVersions) ReadFile(string) (shared.WorldVersion, time.Time, error) {
	return f.version, time.Time{}, nil
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// createBackup は取得を実行し、アーカイブが実際に現れるまで待つ。
//
// CreateBackup が返す Operation は開始時点の断面なので、まだ
// backup_id を持たない。ファイル名は取得が始まってから決まる。
func (h *backupHarness) createBackup(t *testing.T, note string) string {
	t.Helper()

	if _, err := h.client.CreateBackup(context.Background(),
		connect.NewRequest(&mcadminv1.CreateBackupRequest{Note: note})); err != nil {
		t.Fatalf("CreateBackup に失敗: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := h.client.ListBackups(context.Background(),
			connect.NewRequest(&mcadminv1.ListBackupsRequest{}))
		if err == nil && len(res.Msg.GetBackups()) > 0 {
			return res.Msg.GetBackups()[0].GetId()
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("バックアップが作られない")
	return ""
}

// 取得したアーカイブが一覧に出て、実際に展開できる。
func TestBackupCreateAndList(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	res, err := h.client.ListBackups(context.Background(),
		connect.NewRequest(&mcadminv1.ListBackupsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Msg.GetBackups()) != 1 {
		t.Fatalf("%d 件", len(res.Msg.GetBackups()))
	}

	got := res.Msg.GetBackups()[0]
	if got.GetId() != id {
		t.Errorf("ID が %q。%q のはず", got.GetId(), id)
	}
	if got.GetDeclaredVersion() != "26.2" {
		t.Errorf("DeclaredVersion が %q", got.GetDeclaredVersion())
	}
	if got.GetArchiveLevel() != "world" {
		t.Errorf("ArchiveLevel が %q", got.GetArchiveLevel())
	}
	if !got.GetVersion().GetReadable() {
		t.Error("バージョンが読めていない")
	}
	if got.GetSizeBytes() <= 0 {
		t.Errorf("サイズが %d", got.GetSizeBytes())
	}
	if res.Msg.GetDirectory() != h.backupDir {
		t.Errorf("保管先が %q", res.Msg.GetDirectory())
	}
}

// server.properties は API 経由の取得でも決してアーカイブに入らない。
func TestBackupNeverArchivesServerProperties(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	res, err := h.client.ListBackups(context.Background(),
		connect.NewRequest(&mcadminv1.ListBackupsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetBackups()[0].GetId() != id {
		t.Fatalf("ID が %q", res.Msg.GetBackups()[0].GetId())
	}

	for _, root := range res.Msg.GetBackups()[0].GetEntryRoots() {
		if root == "data/server.properties" {
			t.Fatal("server.properties がアーカイブに含まれている")
		}
	}
}

func TestBackupDelete(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	res, err := h.client.DeleteBackup(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteBackupRequest{BackupId: id}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg.GetFreedBytes() <= 0 {
		t.Errorf("解放が %d バイト", res.Msg.GetFreedBytes())
	}
	if _, err := os.Stat(filepath.Join(h.backupDir, id)); !os.IsNotExist(err) {
		t.Error("削除されていない")
	}
}

// 保管先の外を指す識別子は InvalidArgument で断る。
// 内部エラーに丸めると、利用者は打ち間違いに気づけない。
func TestBackupDeleteRejectsTraversal(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)

	_, err := h.client.DeleteBackup(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteBackupRequest{BackupId: "../.env"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("コードが %v。InvalidArgument のはず", connect.CodeOf(err))
	}
}

func TestBackupDeleteMissing(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)

	_, err := h.client.DeleteBackup(context.Background(),
		connect.NewRequest(&mcadminv1.DeleteBackupRequest{
			BackupId: "backup-26.2-world-20260901-000000.zip",
		}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("コードが %v。NotFound のはず", connect.CodeOf(err))
	}
}

func TestBackupRetentionPolicy(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)

	got, err := h.client.GetRetentionPolicy(context.Background(),
		connect.NewRequest(&mcadminv1.GetRetentionPolicyRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Msg.GetPolicy().GetKeepCount() != 10 {
		t.Errorf("KeepCount が %d", got.Msg.GetPolicy().GetKeepCount())
	}

	set, err := h.client.SetRetentionPolicy(context.Background(),
		connect.NewRequest(&mcadminv1.SetRetentionPolicyRequest{
			Policy: &mcadminv1.RetentionPolicy{KeepCount: 3, KeepDays: 7},
		}))
	if err != nil {
		t.Fatal(err)
	}
	if set.Msg.GetPolicy().GetKeepCount() != 3 || set.Msg.GetPolicy().GetKeepDays() != 7 {
		t.Errorf("ポリシーが %v", set.Msg.GetPolicy())
	}
}

// 保持世代数 0 は全損に直結する。InvalidArgument で断る。
func TestBackupRetentionPolicyRejectsZero(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)

	_, err := h.client.SetRetentionPolicy(context.Background(),
		connect.NewRequest(&mcadminv1.SetRetentionPolicyRequest{
			Policy: &mcadminv1.RetentionPolicy{KeepCount: 0},
		}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("コードが %v。InvalidArgument のはず", connect.CodeOf(err))
	}
}

// dry_run は対象だけを返して削除しない。
func TestBackupPruneDryRun(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	h.createBackup(t, "")

	if _, err := h.client.SetRetentionPolicy(context.Background(),
		connect.NewRequest(&mcadminv1.SetRetentionPolicyRequest{
			Policy: &mcadminv1.RetentionPolicy{KeepCount: 1},
		})); err != nil {
		t.Fatal(err)
	}

	res, err := h.client.PruneBackups(context.Background(),
		connect.NewRequest(&mcadminv1.PruneBackupsRequest{DryRun: true}))
	if err != nil {
		t.Fatal(err)
	}
	// 1 件しか無いので、最低 1 世代の保証によって削除対象は出ない。
	if len(res.Msg.GetDeletedIds()) != 0 {
		t.Errorf("削除対象が %v。1 件だけなら残るはず", res.Msg.GetDeletedIds())
	}
}

// 事前確認は副作用を持たず、判断の材料をすべて返す。
func TestPreflightRestoreOverRPC(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	res, err := h.client.PreflightRestore(context.Background(),
		connect.NewRequest(&mcadminv1.PreflightRestoreRequest{BackupId: id}))
	if err != nil {
		t.Fatal(err)
	}

	msg := res.Msg
	if msg.GetCurrentLevel() != "world" {
		t.Errorf("CurrentLevel が %q", msg.GetCurrentLevel())
	}
	if msg.GetConfiguredMcVersion() != "26.2" {
		t.Errorf("ConfiguredMcVersion が %q", msg.GetConfiguredMcVersion())
	}
	if msg.GetVerdict() != mcadminv1.VersionVerdict_VERSION_VERDICT_MATCH {
		t.Errorf("Verdict が %v。一致のはず", msg.GetVerdict())
	}
	if msg.GetLevelNameMismatch() {
		t.Error("名前が食い違っていることになっている")
	}
	if msg.GetRequiresConfirmation() {
		t.Errorf("一致しているのに承諾を求めている。警告: %v", msg.GetWarnings())
	}
	if msg.GetRequiredBytes() <= 0 || msg.GetAvailableBytes() <= 0 {
		t.Errorf("容量が 必要 %d / 空き %d", msg.GetRequiredBytes(), msg.GetAvailableBytes())
	}
	if msg.GetBackup().GetArchiveLevel() != "world" {
		t.Errorf("ArchiveLevel が %q", msg.GetBackup().GetArchiveLevel())
	}
}

// 復元先の名前を打ち間違えたら InvalidArgument で断る。
func TestRestoreRejectsWrongConfirmName(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	_, err := h.client.RestoreBackup(context.Background(),
		connect.NewRequest(&mcadminv1.RestoreBackupRequest{
			BackupId: id, ConfirmLevelName: "wrold",
		}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("コードが %v。InvalidArgument のはず", connect.CodeOf(err))
	}
}

// 実際に復元し、ワールドが退避されて中身が戻ることを確かめる。
func TestRestoreOverRPC(t *testing.T) {
	t.Parallel()

	h := newBackupHarness(t)
	id := h.createBackup(t, "")

	// 取得後にワールドを書き換える。復元で元に戻るはず。
	writeTestFile(t, filepath.Join(h.root, "data", "world", "level.dat"), "壊れた")

	res, err := h.client.RestoreBackup(context.Background(),
		connect.NewRequest(&mcadminv1.RestoreBackupRequest{
			BackupId: id, ConfirmLevelName: "world",
		}))
	if err != nil {
		t.Fatal(err)
	}

	snap := waitOperation(t, h.ops, res.Msg.GetOperation().GetId())
	if snap.State != operation.StateSucceeded {
		t.Fatalf("復元に失敗: %s", snap.ErrorMessage)
	}

	got, err := os.ReadFile(filepath.Join(h.root, "data", "world", "level.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "level" {
		t.Errorf("level.dat が %q。復元されていない", got)
	}

	// 退避したものは自動では消さない。動作を確認してから人が消す。
	quarantine := snap.Attributes["quarantine_path"]
	if quarantine == "" {
		t.Fatal("退避先が記録されていない")
	}
	if _, err := os.Stat(filepath.Join(h.root, "data", quarantine)); err != nil {
		t.Errorf("退避先が残っていない (%s): %v", quarantine, err)
	}
}

// waitOperation は操作が終わるまで待つ。
func waitOperation(t *testing.T, ops *operations.Manager, id string) operation.Snapshot {
	t.Helper()

	opID, err := operation.NewID(id)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := ops.Get(opID); ok && snap.State.IsTerminal() {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
	return operation.Snapshot{}
}
