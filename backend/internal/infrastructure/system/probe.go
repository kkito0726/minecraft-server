package system

import (
	"runtime"
	"sync"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// Probe は port.SystemProbe の実装。
//
// CPU 使用率は累積値の差分でしか出せないため、前回の値を持ち越す。
// 呼ばれた間隔がそのまま測定区間になる。常駐して定期的に測る作りには
// していない。画面を開いていない間も測り続ける理由が無く、4GB の Pi で
// 常駐物を増やしたくないため。
type Probe struct {
	mu      sync.Mutex
	prev    cpuSample
	hasPrev bool

	// now と readers は差し替え可能にしておく。実機の /proc を
	// 触らずに、区間をまたぐ振る舞いをテストするため。
	now      func() time.Time
	readStat func() (string, error)
	readMem  func() (string, error)
	readLoad func() (string, error)
	readDisk func(path string) (port.StorageUsage, error)
}

// New は Probe を作る。
func New() *Probe {
	return &Probe{
		now:      time.Now,
		readStat: readProcStat,
		readMem:  readProcMeminfo,
		readLoad: readProcLoadavg,
		readDisk: readStorage,
	}
}

// Read は path の載っているファイルシステムを対象に 3 つを読む。
func (p *Probe) Read(path string) port.SystemMetrics {
	return port.SystemMetrics{
		CPU:     p.cpu(),
		Memory:  p.memory(),
		Storage: p.storage(path),
	}
}

func (p *Probe) cpu() port.CPUUsage {
	usage := port.CPUUsage{Cores: runtime.NumCPU()}

	// ロードアベレージは単体で意味があるので、読めたら先に入れておく。
	// 使用率が出せなくても、これだけは見せられる。
	if s, err := p.readLoad(); err == nil {
		usage.Load1, usage.Load5, usage.Load15, _ = parseLoadavg(s)
	}

	raw, err := p.readStat()
	if err != nil {
		usage.UnavailableReason = err.Error()
		return usage
	}
	cur, err := parseProcStat(raw, p.now())
	if err != nil {
		usage.UnavailableReason = err.Error()
		return usage
	}

	p.mu.Lock()
	prev, hasPrev := p.prev, p.hasPrev
	p.prev, p.hasPrev = cur, true
	p.mu.Unlock()

	if !hasPrev {
		usage.UnavailableReason = "測定中です（次の取得から出ます）"
		return usage
	}

	percent, window, ok := cpuPercent(prev, cur)
	if !ok {
		usage.UnavailableReason = "前回との差分を取れませんでした"
		return usage
	}

	usage.Available = true
	usage.UsedPercent = percent
	usage.WindowSeconds = window
	return usage
}

func (p *Probe) memory() port.MemoryUsage {
	raw, err := p.readMem()
	if err != nil {
		return port.MemoryUsage{UnavailableReason: err.Error()}
	}
	usage, err := parseMeminfo(raw)
	if err != nil {
		return port.MemoryUsage{UnavailableReason: err.Error()}
	}
	return usage
}

func (p *Probe) storage(path string) port.StorageUsage {
	usage, err := p.readDisk(path)
	if err != nil {
		return port.StorageUsage{Path: path, UnavailableReason: err.Error()}
	}
	return usage
}
