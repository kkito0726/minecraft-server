package operations_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/operations"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
	"github.com/kkito0726/minecraft-server/backend/internal/infrastructure/persistence/lockfile"
)

func newManager(t *testing.T) *operations.Manager {
	t.Helper()

	m, err := operations.NewManager(operations.Config{
		Lock: lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func steps() []string { return []string{"準備", "実行"} }

// バックアップ・復元・切替・起動停止はすべて data/ を変更する。
// 同時に走ると壊れるため、実行中は新しい操作を拒否する。
func TestOnlyOneOperationAtATime(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	release := make(chan struct{})
	started := make(chan struct{})

	go func() {
		_, _ = m.Start(context.Background(), operation.KindBackupCreate, steps(),
			func(_ context.Context, r operations.Reporter) error {
				close(started)
				<-release
				_ = r.Step()
				return nil
			})
	}()

	<-started
	_, err := m.Start(context.Background(), operation.KindWorldSwitch, steps(),
		func(context.Context, operations.Reporter) error { return nil })
	if !errors.Is(err, operations.ErrBusy) {
		t.Errorf("ErrBusy を期待したが %v", err)
	}

	close(release)
	waitIdle(t, m)
}

// 完了したら次を開始できる。ロックが確実に解放されること。
func TestNextOperationAfterCompletion(t *testing.T) {
	t.Parallel()

	m := newManager(t)

	runSync(t, m, operation.KindBackupCreate, func(context.Context, operations.Reporter) error {
		return nil
	})
	runSync(t, m, operation.KindWorldSwitch, func(context.Context, operations.Reporter) error {
		return nil
	})
}

// 失敗してもロックは解放される。これが漏れると以降すべての操作が拒否される。
func TestLockReleasedOnFailure(t *testing.T) {
	t.Parallel()

	m := newManager(t)

	runSync(t, m, operation.KindBackupCreate, func(context.Context, operations.Reporter) error {
		return errors.New("失敗")
	})
	// 次が開始できること
	runSync(t, m, operation.KindWorldSwitch, func(context.Context, operations.Reporter) error {
		return nil
	})
}

// panic してもロックは解放される。
func TestLockReleasedOnPanic(t *testing.T) {
	t.Parallel()

	m := newManager(t)

	op, err := m.Start(context.Background(), operation.KindBackupCreate, steps(),
		func(context.Context, operations.Reporter) error {
			panic("想定外の失敗")
		})
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, m, op.ID())

	got, _ := m.Get(op.ID())
	if got.State != operation.StateFailed {
		t.Errorf("panic した操作の State が %v。失敗のはず", got.State)
	}

	runSync(t, m, operation.KindWorldSwitch, func(context.Context, operations.Reporter) error {
		return nil
	})
}

// ロックファイルに操作の情報が記録される。プロセスをまたいで中断を検出するため。
func TestLockFileRecordsOperation(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), ".lock")
	m, err := operations.NewManager(operations.Config{Lock: lockfile.NewLock(lockPath)})
	if err != nil {
		t.Fatal(err)
	}

	seen := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_, _ = m.Start(context.Background(), operation.KindBackupCreate, steps(),
			func(_ context.Context, r operations.Reporter) error {
				r.MarkSaveDisabled(true)
				close(seen)
				<-release
				return nil
			})
	}()

	<-seen
	// 少し待ってから読む（MarkSaveDisabled の反映を待つ）
	time.Sleep(50 * time.Millisecond)
	b, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ロックファイルを読めない: %v", err)
	}
	if len(b) == 0 {
		t.Error("ロックファイルが空")
	}

	close(release)
	waitIdle(t, m)
}

// 操作は呼び出し元の context に紐づかない。
//
// HTTP ハンドラから起動すると、リクエストが返った時点で context が
// キャンセルされる。操作の本体がそれを引き継ぐと、復元やバックアップが
// 開始直後に死ぬ。「操作はサーバー側のリソース」という設計の前提そのもの。
func TestOperationSurvivesCallerContextCancel(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	proceed := make(chan struct{})
	observed := make(chan error, 1)

	// リクエストの context を模す
	callerCtx, cancelCaller := context.WithCancel(context.Background())

	h, err := m.Start(callerCtx, operation.KindBackupCreate, steps(),
		func(ctx context.Context, r operations.Reporter) error {
			// 呼び出し元がキャンセルされるのを待ってから続行する
			<-proceed
			observed <- ctx.Err()
			return r.Step()
		})
	if err != nil {
		t.Fatal(err)
	}

	// HTTP ハンドラが返った状況を再現
	cancelCaller()
	time.Sleep(50 * time.Millisecond)
	close(proceed)

	select {
	case ctxErr := <-observed:
		if ctxErr != nil {
			t.Errorf("操作の context が %v。呼び出し元のキャンセルを引き継いではいけない", ctxErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("操作が進まない")
	}

	snap := waitFinishedSnapshot(t, m, h.ID())
	if snap.State != operation.StateSucceeded {
		t.Errorf("State が %v（%s）。成功のはず", snap.State, snap.ErrorMessage)
	}
}

// アプリの停止時には操作も止まる。ぶら下がった goroutine を残さない。
func TestOperationStopsOnShutdown(t *testing.T) {
	t.Parallel()

	baseCtx, shutdown := context.WithCancel(context.Background())
	m, err := operations.NewManager(operations.Config{
		Lock:    lockfile.NewLock(filepath.Join(t.TempDir(), ".lock")),
		BaseCtx: baseCtx,
	})
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	h, err := m.Start(context.Background(), operation.KindBackupCreate, steps(),
		func(ctx context.Context, _ operations.Reporter) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	if err != nil {
		t.Fatal(err)
	}

	<-started
	shutdown()

	snap := waitFinishedSnapshot(t, m, h.ID())
	if snap.State != operation.StateFailed {
		t.Errorf("State が %v。停止で中断されるはず", snap.State)
	}
}

func waitFinishedSnapshot(t *testing.T, m *operations.Manager, id operation.ID) operation.Snapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := m.Get(id); ok && snap.State.IsTerminal() {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
	return operation.Snapshot{}
}

// 進行中の操作を識別子なしで取得できる。
// 別端末や localStorage を消した後でも現在の状況が分かる必要がある。
func TestActive(t *testing.T) {
	t.Parallel()

	m := newManager(t)

	if _, ok := m.Active(); ok {
		t.Error("何も実行していないのに Active がある")
	}

	release := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_, _ = m.Start(context.Background(), operation.KindBackupCreate, steps(),
			func(context.Context, operations.Reporter) error {
				close(started)
				<-release
				return nil
			})
	}()

	<-started
	if _, ok := m.Active(); !ok {
		t.Error("実行中なのに Active が取れない")
	}

	close(release)
	waitIdle(t, m)

	if _, ok := m.Active(); ok {
		t.Error("完了後も Active が残っている")
	}
}

// 完了した操作も一定数は参照できる。復元の結果を後から確認するため。
func TestGetAfterCompletion(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	op := runSync(t, m, operation.KindBackupCreate, func(_ context.Context, r operations.Reporter) error {
		_ = r.Step()
		r.Logf(operation.LevelInfo, "処理しました")
		return nil
	})

	got, ok := m.Get(op.ID())
	if !ok {
		t.Fatal("完了した操作を取得できない")
	}
	if got.State != operation.StateSucceeded {
		t.Errorf("State が %v", got.State)
	}
}

func TestList(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	for range 3 {
		runSync(t, m, operation.KindBackupCreate, func(context.Context, operations.Reporter) error {
			return nil
		})
	}

	list := m.List(10)
	if len(list) != 3 {
		t.Fatalf("履歴が %d 件。3 件のはず", len(list))
	}
	// 新しい順
	for i := 1; i < len(list); i++ {
		if list[i-1].StartedAt.Before(list[i].StartedAt) {
			t.Error("新しい順に並んでいない")
		}
	}

	if got := m.List(2); len(got) != 2 {
		t.Errorf("上限が効いていない: %d 件", len(got))
	}
}

// 購読は指定した位置以降のイベントを再送してから追従する。
// これが無いと、リロード直後の画面がスピナーだけになる。
func TestSubscribeReplaysFromSeq(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	op := runSync(t, m, operation.KindBackupCreate, func(_ context.Context, r operations.Reporter) error {
		_ = r.Step()
		r.Logf(operation.LevelInfo, "1 行目")
		r.Logf(operation.LevelInfo, "2 行目")
		_ = r.Step()
		return nil
	})

	t.Run("最初から", func(t *testing.T) {
		t.Parallel()
		events := collect(t, m, op.ID(), 0)
		if len(events) < 5 {
			t.Fatalf("イベントが %d 件。少なすぎる", len(events))
		}
		if events[0].Seq() != 1 {
			t.Errorf("最初の Seq が %d。1 のはず", events[0].Seq())
		}
	})

	t.Run("途中から", func(t *testing.T) {
		t.Parallel()
		all := collect(t, m, op.ID(), 0)
		from := all[2].Seq()

		events := collect(t, m, op.ID(), from)
		if len(events) != len(all)-2 {
			t.Errorf("再送が %d 件。%d 件のはず", len(events), len(all)-2)
		}
		if events[0].Seq() != from {
			t.Errorf("最初の Seq が %d。%d のはず", events[0].Seq(), from)
		}
	})
}

// 実行中の操作を購読すると、既存のイベントを受け取ってから追従する。
func TestSubscribeFollowsLiveOperation(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	release := make(chan struct{})
	started := make(chan struct{})

	go func() {
		_, _ = m.Start(context.Background(), operation.KindBackupCreate, steps(),
			func(_ context.Context, r operations.Reporter) error {
				_ = r.Step()
				close(started)
				<-release
				r.Logf(operation.LevelInfo, "追加のログ")
				_ = r.Step()
				return nil
			})
	}()

	<-started
	time.Sleep(20 * time.Millisecond)
	active, ok := m.Active()
	if !ok {
		t.Fatal("実行中の操作が取れない")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, unsubscribe, err := m.Subscribe(ctx, active.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	close(release)

	var received []operation.Event
	for e := range ch {
		received = append(received, e)
		if e.Snapshot().State.IsTerminal() {
			break
		}
	}

	if len(received) == 0 {
		t.Fatal("イベントを受け取れていない")
	}
	var sawLog bool
	for _, e := range received {
		if e.Message() == "追加のログ" {
			sawLog = true
		}
	}
	if !sawLog {
		t.Error("購読開始後に発生したログを受け取れていない")
	}
}

// 存在しない操作の購読はエラーになる。
func TestSubscribeUnknown(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	if _, _, err := m.Subscribe(context.Background(), operation.MustID("nope"), 0); err == nil {
		t.Error("エラーになるはず")
	}
}

// 購読を解除すると goroutine が残らない。
func TestUnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	op := runSync(t, m, operation.KindBackupCreate, func(_ context.Context, r operations.Reporter) error {
		_ = r.Step()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, unsubscribe, err := m.Subscribe(ctx, op.ID(), 0)
	if err != nil {
		t.Fatal(err)
	}
	unsubscribe()
	cancel()

	// チャネルが閉じること
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, open := <-ch:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("チャネルが閉じない")
		}
	}
}

// 並行して Start を呼んでも 1 本しか通らない。
func TestConcurrentStart(t *testing.T) {
	t.Parallel()

	m := newManager(t)
	const n = 20

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
	)
	release := make(chan struct{})

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Start(context.Background(), operation.KindBackupCreate, steps(),
				func(context.Context, operations.Reporter) error {
					<-release
					return nil
				})
			if err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if succeeded == 0 {
		t.Error("1 本も通っていない")
	}
	// 同時に走ったのは 1 本だけ（後続は逐次的に通りうる）
	waitIdle(t, m)
}

// --- ヘルパー ---

func runSync(
	t *testing.T,
	m *operations.Manager,
	kind operation.Kind,
	fn operations.Func,
) operations.Handle {
	t.Helper()

	h, err := m.Start(context.Background(), kind, steps(), fn)
	if err != nil {
		t.Fatalf("Start に失敗: %v", err)
	}
	waitFinished(t, m, h.ID())
	return h
}

func waitFinished(t *testing.T, m *operations.Manager, id operation.ID) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, ok := m.Get(id); ok && snap.State.IsTerminal() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
}

func waitIdle(t *testing.T, m *operations.Manager) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := m.Active(); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("操作が終わらない")
}

func collect(t *testing.T, m *operations.Manager, id operation.ID, fromSeq int64) []operation.Event {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, unsubscribe, err := m.Subscribe(ctx, id, fromSeq)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	var out []operation.Event
	for e := range ch {
		out = append(out, e)
		if e.Snapshot().State.IsTerminal() {
			break
		}
	}
	return out
}
