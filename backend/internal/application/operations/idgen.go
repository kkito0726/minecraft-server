package operations

import (
	"fmt"
	"sync/atomic"

	"github.com/kkito0726/minecraft-server/backend/internal/domain/operation"
)

// timeIDGenerator は時刻と連番から識別子を作る。
//
// 時刻だけだと同一ミリ秒での衝突がありうるため連番を足す。
// 操作は同時に 1 つしか走らないので、これで十分に一意になる。
type timeIDGenerator struct {
	clock   Clock
	counter atomic.Int64
}

func (g *timeIDGenerator) Next() operation.ID {
	n := g.counter.Add(1)
	return operation.MustID(fmt.Sprintf("%s-%04d", g.clock.Now().Format("20060102-150405.000"), n))
}

var _ IDGenerator = (*timeIDGenerator)(nil)
