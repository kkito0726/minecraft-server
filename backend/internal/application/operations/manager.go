// Package operations は操作の実行と進捗の配信を受け持つ。
//
// data/ を変更する操作（バックアップ・復元・ワールド切替・起動停止）は
// 同時にひとつしか走らせない。並行して走ると region ファイルの
// 書き込みが競合してワールドが壊れる。
//
// 排他は二段構え。プロセス内の Mutex と、data/ 配下のロックファイル。
// 前者だけでは systemd による再起動をまたげず、
// 「バックアップが中断された = save-off が残っているかもしれない」
// という事実も失われる。
package operations

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// ErrBusy は他の操作が実行中であることを表す。
var ErrBusy = errors.New("他の操作が実行中です")

// defaultHistoryLimit は保持する完了済み操作の数。
// 復元の結果を後から確認できる程度に持つ。
const defaultHistoryLimit = 100

// defaultListLimit は List の既定件数。
const defaultListLimit = 50

// Func は操作の本体。Reporter を通じて進捗を報告する。
type Func func(ctx context.Context, r Reporter) error

// Handle は開始した操作の参照。
type Handle struct {
	snapshot operation.Snapshot
}

// ID は操作の識別子を返す。
func (h Handle) ID() operation.ID { return h.snapshot.ID }

// Snapshot は開始時点の状態を返す。
func (h Handle) Snapshot() operation.Snapshot { return h.snapshot }

// Clock は現在時刻。テストで固定できるようにしてある。
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// IDGenerator は操作の識別子を作る。
type IDGenerator interface {
	Next() operation.ID
}

// Config は Manager の設定。
type Config struct {
	// Lock はプロセスをまたぐ排他。
	Lock port.OperationLock
	// BaseCtx は操作の寿命を決める context。
	//
	// 操作は呼び出し元（HTTP リクエスト）の context を引き継がない。
	// 引き継ぐとレスポンスを返した時点でキャンセルされ、復元や
	// バックアップが開始直後に死ぬ。アプリの停止でだけ中断させる。
	// nil なら context.Background()。
	BaseCtx context.Context
	// HistoryLimit は保持する完了済み操作の数。0 なら既定値。
	HistoryLimit int
	// Clock は現在時刻。nil ならシステム時計。
	Clock Clock
	// IDs は識別子の生成器。nil なら時刻ベースの既定実装。
	IDs IDGenerator
}

// Manager は操作の実行と進捗の配信を管理する。
type Manager struct {
	cfg Config
	// baseCtx は操作の寿命。呼び出し元の context とは切り離す。
	baseCtx context.Context
	clock   Clock
	ids     IDGenerator

	// mu は状態全体を守る。操作の本体は mu を持たずに実行する。
	mu sync.Mutex
	// busy は実行中の操作があるか。
	busy bool
	// lock は実行中のロック。
	lock port.LockHandle
	// lockMeta は実行中のロックの内容。書き換えのために保持する。
	lockMeta port.LockMeta
	// active は実行中の操作。
	active *operation.Operation
	// history は完了済みの操作。新しいものが末尾。
	history []*operation.Operation
	// subscribers は購読者。操作の識別子ごとに保持する。
	subscribers map[string][]*subscriber
}

// NewManager は Manager を作る。
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Lock == nil {
		return nil, errors.New("ロックが指定されていません")
	}
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = defaultHistoryLimit
	}

	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	ids := cfg.IDs
	if ids == nil {
		ids = &timeIDGenerator{clock: clock}
	}

	baseCtx := cfg.BaseCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	return &Manager{
		cfg:         cfg,
		baseCtx:     baseCtx,
		clock:       clock,
		ids:         ids,
		subscribers: map[string][]*subscriber{},
	}, nil
}

// Start は操作を開始する。実行中の操作があれば ErrBusy を返す。
//
// 本体は別の goroutine で走り、Start は開始した時点で返る。
// 呼び出し側は Handle の識別子で進捗を購読する。
//
// ctx は開始の可否を判断するまでにしか使わない。**操作の本体は
// ctx を引き継がない。** HTTP ハンドラから呼ぶとレスポンスを返した
// 時点でキャンセルされ、復元やバックアップが開始直後に死ぬためで、
// 「操作はサーバー側のリソース」という設計の前提そのものにあたる。
// 本体は BaseCtx（アプリの寿命）に紐づく。
func (m *Manager) Start(
	ctx context.Context,
	kind operation.Kind,
	stepNames []string,
	fn Func,
) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}

	// スナップショットはロック下で取る。goroutine を起こしてから読むと、
	// 本体が既にステップを進めていて競合する。
	op, snapshot, err := m.begin(kind, stepNames)
	if err != nil {
		return Handle{}, err
	}

	go m.run(m.baseCtx, op, fn)
	return Handle{snapshot: snapshot}, nil
}

// begin はロックを取って操作を作り、開始時点のスナップショットも返す。
func (m *Manager) begin(
	kind operation.Kind,
	stepNames []string,
) (*operation.Operation, operation.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.busy {
		return nil, operation.Snapshot{}, fmt.Errorf("%w（%s）", ErrBusy, m.active.Kind())
	}

	id := m.ids.Next()
	op, err := operation.New(id, kind, stepNames, m.clock.Now())
	if err != nil {
		return nil, operation.Snapshot{}, fmt.Errorf("操作を作れません: %w", err)
	}

	meta := port.LockMeta{
		OperationID: id.String(),
		Kind:        kind.String(),
		PID:         processID(),
		StartedAt:   m.clock.Now(),
	}
	lock, err := m.cfg.Lock.Acquire(meta)
	if err != nil {
		if errors.Is(err, port.ErrLocked) {
			return nil, operation.Snapshot{}, fmt.Errorf("%w（%s）", ErrBusy, kind)
		}
		return nil, operation.Snapshot{}, err
	}

	m.busy = true
	m.lock = lock
	m.lockMeta = meta
	m.active = op
	return op, m.snapshotOf(op), nil
}

// run は操作の本体を実行し、終了処理を行う。
//
// panic しても必ずロックを解放する。ここが漏れると、
// 以降すべての操作が ErrBusy で拒否され続ける。
func (m *Manager) run(ctx context.Context, op *operation.Operation, fn Func) {
	reporter := &reporter{manager: m, op: op}

	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("操作が異常終了しました: %v", r)
			}
		}()
		return fn(ctx, reporter)
	}()

	m.finish(op, err)
}

func (m *Manager) finish(op *operation.Operation, err error) {
	m.mu.Lock()

	now := m.clock.Now()
	if err != nil {
		_ = op.Fail(now, errorCodeOf(err), err.Error())
	} else {
		_ = op.Succeed(now)
	}

	events := op.Events()
	m.releaseLocked()
	m.pushHistoryLocked(op)

	// 購読者を取り出したうえで登録から外す。以降の publish が
	// 閉じたチャネルを掴まないようにするため。
	subs := m.subscribersOf(op.ID())
	delete(m.subscribers, op.ID().String())

	m.mu.Unlock()

	// 通知と閉鎖は mu を持たずに行う。購読者が遅くても他の操作を止めない。
	for _, s := range subs {
		if len(events) > 0 {
			s.send(events[len(events)-1])
		}
		s.close()
	}
}

// releaseLocked はロックを解放する。mu を保持した状態で呼ぶ。
func (m *Manager) releaseLocked() {
	if m.lock != nil {
		_ = m.lock.Release()
		m.lock = nil
		m.lockMeta = port.LockMeta{}
	}
	m.busy = false
	m.active = nil
}

func (m *Manager) pushHistoryLocked(op *operation.Operation) {
	m.history = append(m.history, op)
	if len(m.history) > m.cfg.HistoryLimit {
		m.history = m.history[len(m.history)-m.cfg.HistoryLimit:]
	}
}

// Active は実行中の操作を返す。
//
// 識別子を保持していないクライアント（別端末、localStorage を消した場合）が
// 現在の状況を知るための入口。
func (m *Manager) Active() (operation.Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return operation.Snapshot{}, false
	}
	return m.snapshotOf(m.active), true
}

// Get は識別子で操作を取得する。完了済みも履歴の範囲で参照できる。
func (m *Manager) Get(id operation.ID) (operation.Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	op := m.findLocked(id)
	if op == nil {
		return operation.Snapshot{}, false
	}
	return m.snapshotOf(op), true
}

// List は直近の操作を新しい順に返す。
func (m *Manager) List(limit int) []operation.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 {
		limit = defaultListLimit
	}

	all := make([]*operation.Operation, 0, len(m.history)+1)
	if m.active != nil {
		all = append(all, m.active)
	}
	for i := len(m.history) - 1; i >= 0; i-- {
		all = append(all, m.history[i])
	}

	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]operation.Snapshot, len(all))
	for i, op := range all {
		out[i] = m.snapshotOf(op)
	}
	return out
}

func (m *Manager) findLocked(id operation.ID) *operation.Operation {
	if m.active != nil && m.active.ID() == id {
		return m.active
	}
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].ID() == id {
			return m.history[i]
		}
	}
	return nil
}

func (m *Manager) snapshotOf(op *operation.Operation) operation.Snapshot {
	events := op.Events()
	if len(events) == 0 {
		// まだイベントが無い（開始直後）。現在の状態を組み立てる。
		return operation.Snapshot{
			ID:        op.ID(),
			Kind:      op.Kind(),
			State:     op.State(),
			StartedAt: op.StartedAt(),
			StepTotal: op.StepTotal(),
		}
	}
	return events[len(events)-1].Snapshot()
}

// errorCodeOf はエラーから安定した識別子を導く。
// 画面の分岐に使うため、日本語のメッセージには依存させない。
func errorCodeOf(err error) string {
	switch {
	case errors.Is(err, ErrBusy):
		return "E_BUSY"
	case errors.Is(err, context.Canceled):
		return "E_CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "E_TIMEOUT"
	default:
		return "E_INTERNAL"
	}
}
