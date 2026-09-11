package reconcile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/reconcile"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/server"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

// fakeConsole は save-on の送信回数を数える。
type fakeConsole struct {
	mu       sync.Mutex
	saveOnN  int
	failWith error
}

func (f *fakeConsole) SaveOff(context.Context) error { return nil }
func (f *fakeConsole) SaveAll(context.Context) error { return nil }

func (f *fakeConsole) SaveOn(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveOnN++
	return f.failWith
}

func (f *fakeConsole) PlayerCount(context.Context) (int, int, bool, error) {
	return 0, 0, false, nil
}

func (f *fakeConsole) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saveOnN
}

// fakeStatus は healthy の遷移を再現する。
type fakeStatus struct {
	mu       sync.Mutex
	sequence []bool
	index    int
}

func (f *fakeStatus) Healthy(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.index >= len(f.sequence) {
		if len(f.sequence) == 0 {
			return false, nil
		}
		return f.sequence[len(f.sequence)-1], nil
	}
	v := f.sequence[f.index]
	f.index++
	return v, nil
}

func newReconciler(t *testing.T, console *fakeConsole, status *fakeStatus, lockPath string) *reconcile.Reconciler {
	t.Helper()

	r, err := reconcile.New(reconcile.Config{
		Console:  console,
		Health:   status,
		Lock:     lockfile.NewLock(lockPath),
		Interval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// 起動時に無条件で save-on を送る。
//
// save-off を残したままプロセスが落ちると、以降の変更がディスクに
// 書かれないのに症状が何も出ない。RCON には保存状態を問い合わせる
// 手段がないため、状態を照会せず冪等な save-on を送る。
func TestRunOnceSendsSaveOnUnconditionally(t *testing.T) {
	t.Parallel()

	console := &fakeConsole{}
	r := newReconciler(t, console, &fakeStatus{}, filepath.Join(t.TempDir(), ".lock"))

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce に失敗: %v", err)
	}
	if console.count() != 1 {
		t.Errorf("save-on が %d 回。1 回のはず", console.count())
	}
}

// コンテナが停止していると送信は失敗するが、それを異常にしない。
// バックエンドは MC より先に起動しうる。
func TestRunOnceToleratesFailure(t *testing.T) {
	t.Parallel()

	console := &fakeConsole{failWith: errors.New("コンテナが見つかりません")}
	r := newReconciler(t, console, &fakeStatus{}, filepath.Join(t.TempDir(), ".lock"))

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Errorf("送信失敗を異常にしないはず: %v", err)
	}
	if console.count() != 1 {
		t.Errorf("送信を試みていない")
	}
}

// 前回の実行が操作の途中で終了していたら検出する。
func TestRunOnceDetectsInterruption(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), ".lock")
	// 存在しないプロセスのロックを置く
	h, err := lockfile.Acquire(lockPath, lockfile.Meta{
		OperationID:  "op-1",
		Kind:         "バックアップの取得",
		PID:          999999,
		StartedAt:    time.Now(),
		SaveDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = h // Release せずに残す

	console := &fakeConsole{}
	r := newReconciler(t, console, &fakeStatus{}, lockPath)

	result, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.InterruptedDetected {
		t.Error("中断を検出できていない")
	}
	// stale なロックは削除する。残すと以降すべての操作が拒否される。
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Error("stale なロックが削除されていない")
	}
}

// 生きているプロセスのロックは消さない。多重起動での誤削除を防ぐ。
func TestRunOnceKeepsLiveLock(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), ".lock")
	h, err := lockfile.Acquire(lockPath, lockfile.Meta{
		OperationID: "op-1",
		PID:         os.Getpid(),
		StartedAt:   time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	r := newReconciler(t, &fakeConsole{}, &fakeStatus{}, lockPath)

	result, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.InterruptedDetected {
		t.Error("生きているロックを中断と判定した")
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Error("生きているロックを削除した")
	}
}

// ロックが無ければ中断ではない。
func TestRunOnceWithoutLock(t *testing.T) {
	t.Parallel()

	r := newReconciler(t, &fakeConsole{}, &fakeStatus{}, filepath.Join(t.TempDir(), ".lock"))

	result, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.InterruptedDetected {
		t.Error("ロックが無いのに中断と判定した")
	}
}

// 非 healthy から healthy への遷移ごとに save-on を再送する。
//
// 起動時の送信はコンテナが未起動なら失敗する。その補償として、
// サーバーが応答できるようになった時点で送り直す。
func TestLoopResendsOnHealthyTransition(t *testing.T) {
	t.Parallel()

	console := &fakeConsole{}
	// 不合格 → 不合格 → 合格 → 合格 → 合格
	status := &fakeStatus{sequence: []bool{false, false, true, true, true}}
	r := newReconciler(t, console, status, filepath.Join(t.TempDir(), ".lock"))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Loop(ctx)

	// 遷移は 1 回だけ。合格が続く間は再送しない。
	if got := console.count(); got != 1 {
		t.Errorf("save-on が %d 回。遷移 1 回分のはず", got)
	}
}

// 合格 → 不合格 → 合格 なら 2 回送る。再起動のたびに送り直す。
func TestLoopResendsOnEachTransition(t *testing.T) {
	t.Parallel()

	console := &fakeConsole{}
	status := &fakeStatus{sequence: []bool{true, false, true, true}}
	r := newReconciler(t, console, status, filepath.Join(t.TempDir(), ".lock"))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Loop(ctx)

	if got := console.count(); got != 2 {
		t.Errorf("save-on が %d 回。遷移 2 回分のはず", got)
	}
}

// 合格が続くだけなら送らない。無駄な RCON 呼び出しを避ける。
func TestLoopDoesNotResendWhileHealthy(t *testing.T) {
	t.Parallel()

	console := &fakeConsole{}
	status := &fakeStatus{sequence: []bool{true, true, true, true, true}}
	r := newReconciler(t, console, status, filepath.Join(t.TempDir(), ".lock"))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r.Loop(ctx)

	if got := console.count(); got != 1 {
		t.Errorf("save-on が %d 回。初回の 1 回だけのはず", got)
	}
}

// Loop は context のキャンセルで止まる。
func TestLoopStopsOnCancel(t *testing.T) {
	t.Parallel()

	r := newReconciler(t, &fakeConsole{}, &fakeStatus{}, filepath.Join(t.TempDir(), ".lock"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Loop(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop が止まらない")
	}
}

// 設定が足りなければ作れない。
func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	if _, err := reconcile.New(reconcile.Config{}); err == nil {
		t.Error("エラーになるはず")
	}
}

var _ = server.SavingAssumedOn

/*
バックアップの最中に save-on を送り返してはいけない。

hot バックアップは save-off でワールドの保存を止めてから zip を固める。
その間コンテナは healthy のままなので通常は遷移が起きないが、負荷で
ヘルスチェックが一度落ちて戻ると「非 healthy → healthy」になる。
Pi では I/O 負荷でこれが起こりうる。

そこで save-on を送ると、**zip を書いている最中に保存が再開される**。
書き込み途中の region ファイルが取り込まれ、静かに壊れたバックアップが
できあがる。save-off がそもそも防いでいたはずのものになる。

生きているロックが「保存を止めている」と言っている間は送らない。
*/
func TestLoopDoesNotResendWhileBackupHoldsSaveOff(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), ".lock")
	h, err := lockfile.Acquire(lockPath, lockfile.Meta{
		OperationID:  "op-1",
		Kind:         "BACKUP_CREATE",
		PID:          os.Getpid(),
		StartedAt:    time.Now(),
		SaveDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	console := &fakeConsole{}
	// 不合格 → 合格。負荷でヘルスチェックが一度落ちて戻った状況。
	status := &fakeStatus{sequence: []bool{false, true, true, true}}
	r := newReconciler(t, console, status, lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Loop(ctx)

	if got := console.count(); got != 0 {
		t.Errorf("バックアップ中に save-on を %d 回送った", got)
	}
}

// 保存を止めていない操作（切替や復元の待ち時間）なら送ってよい。
// 送らないと、中断された save-off が回復されないまま残る。
func TestLoopResendsWhenLockDoesNotDisableSaving(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), ".lock")
	h, err := lockfile.Acquire(lockPath, lockfile.Meta{
		OperationID: "op-1",
		Kind:        "WORLD_SWITCH",
		PID:         os.Getpid(),
		StartedAt:   time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Release() }()

	console := &fakeConsole{}
	status := &fakeStatus{sequence: []bool{false, true, true, true}}
	r := newReconciler(t, console, status, lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.Loop(ctx)

	if console.count() == 0 {
		t.Error("保存を止めていない操作の最中にも送られなかった")
	}
}
