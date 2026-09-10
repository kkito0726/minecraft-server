package operations

import (
	"context"
	"fmt"
	"sync"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// liveBuffer は購読開始後に発生するイベント用のバッファ。
//
// 遅い購読者が操作の進行を止めないための余裕。
const liveBuffer = 256

// subscriber は 1 つの購読。
//
// チャネルへの送信と閉鎖を自身の Mutex で守る。Manager の Mutex とは
// 別にしているのは、購読者が 1 つ詰まっても他の操作を止めないため。
//
// 送信は必ず非ブロッキングにしてある。ブロッキング送信をロック下で行うと、
// 受け取らない購読者がいるだけで全体が停止する。
type subscriber struct {
	mu     sync.Mutex
	ch     chan operation.Event
	closed bool
}

// send はイベントを配信する。閉じている、またはバッファが溢れていれば偽を返す。
func (s *subscriber) send(e operation.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	select {
	case s.ch <- e:
		return true
	default:
		// 溢れた。取りこぼしたまま配信を続けるとクライアントが
		// 状態のずれに気づけないため、ここで打ち切る。
		// クライアントは最後に受け取った seq から再購読する。
		return false
	}
}

// close はチャネルを閉じる。二重に呼んでも安全。
func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// Subscribe は操作の進捗を購読する。
//
// fromSeq 以降の記録済みイベントを再送してから、以後のイベントを配信する。
// 再送が無いと、ブラウザを再読み込みした直後の画面がスピナーだけになる。
//
// 返り値のチャネルは操作の終了時、または unsubscribe / ctx のキャンセルで閉じる。
func (m *Manager) Subscribe(
	ctx context.Context,
	id operation.ID,
	fromSeq int64,
) (<-chan operation.Event, func(), error) {
	m.mu.Lock()

	op := m.findLocked(id)
	if op == nil {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("操作が見つかりません: %s", id)
	}

	backlog := eventsFrom(op.Events(), fromSeq)
	finished := op.IsFinished()

	// 再送分が必ず収まる大きさにする。これで送信を常に非ブロッキングにでき、
	// 「閉じたチャネルへ送る」競合を構造的に避けられる。
	sub := &subscriber{ch: make(chan operation.Event, len(backlog)+liveBuffer)}

	// 記録済みのイベントをロック下で流し込む。この間に新しいイベントが
	// 割り込むと順序が乱れるため、購読者の登録より先に済ませる。
	for _, e := range backlog {
		sub.send(e)
	}
	if !finished {
		m.subscribers[id.String()] = append(m.subscribers[id.String()], sub)
	}
	m.mu.Unlock()

	if finished {
		// 完了済みの操作。再送だけして閉じる。
		sub.close()
		return sub.ch, func() {}, nil
	}

	// ctx のキャンセルで購読を解除する。
	stop := make(chan struct{})
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			close(stop)
			m.removeSubscriber(id, sub)
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-stop:
		}
	}()

	return sub.ch, unsubscribe, nil
}

// eventsFrom は指定した位置以降のイベントを返す。
// fromSeq が 0 以下なら全件。
func eventsFrom(events []operation.Event, fromSeq int64) []operation.Event {
	if fromSeq <= 0 {
		return events
	}
	for i, e := range events {
		if e.Seq() >= fromSeq {
			return events[i:]
		}
	}
	return nil
}

// publish は購読者へイベントを配信する。Manager の mu を保持せずに呼ぶ。
func (m *Manager) publish(id operation.ID, e operation.Event) {
	m.mu.Lock()
	subs := m.subscribersOf(id)
	m.mu.Unlock()

	for _, s := range subs {
		s.send(e)
	}
}

// subscribersOf は購読者の複製を返す。mu を保持した状態で呼ぶ。
func (m *Manager) subscribersOf(id operation.ID) []*subscriber {
	src := m.subscribers[id.String()]
	out := make([]*subscriber, len(src))
	copy(out, src)
	return out
}

func (m *Manager) removeSubscriber(id operation.ID, sub *subscriber) {
	m.mu.Lock()
	key := id.String()
	subs := m.subscribers[key]
	for i, s := range subs {
		if s == sub {
			m.subscribers[key] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(m.subscribers[key]) == 0 {
		delete(m.subscribers, key)
	}
	m.mu.Unlock()

	sub.close()
}
