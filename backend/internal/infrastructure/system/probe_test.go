package system

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// newTestProbe は /proc を触らない Probe を作る。
// 時刻は呼ばれるたびに 5 秒進める（画面の更新間隔と同じ）。
func newTestProbe(stat, mem, load string, disk port.StorageUsage) *Probe {
	tick := time.Unix(0, 0)
	return &Probe{
		now: func() time.Time {
			tick = tick.Add(5 * time.Second)
			return tick
		},
		readStat: func() (string, error) { return stat, nil },
		readMem:  func() (string, error) { return mem, nil },
		readLoad: func() (string, error) { return load, nil },
		readDisk: func(string) (port.StorageUsage, error) { return disk, nil },
	}
}

// 1 回目は差分が取れない。ここで前回値のつもりの 0 と引き算して
// 「起動からの平均」を出してしまうと、常に同じ数字が出る壊れ方をする。
func TestCPUNeedsTwoSamples(t *testing.T) {
	t.Parallel()

	p := newTestProbe(procStatSample, meminfoSample, "0.1 0.2 0.3 1/2 3", port.StorageUsage{})

	first := p.Read("/tmp").CPU
	if first.Available {
		t.Error("1 回目から使用率が出ている")
	}
	if !strings.Contains(first.UnavailableReason, "測定中") {
		t.Errorf("理由が %q", first.UnavailableReason)
	}

	// 2 回目は同じ内容を返すので差分が 0 になる。値が動く様子は
	// cpuPercent のテストで見ているので、ここでは差分の有無だけを見る。
	p.readStat = func() (string, error) {
		return "cpu  20 20 30 500 50 6 7 0 0 0\n", nil
	}
	second := p.Read("/tmp").CPU
	if !second.Available {
		t.Fatalf("2 回目も出ない: %q", second.UnavailableReason)
	}
	if second.WindowSeconds != 5 {
		t.Errorf("区間が %v 秒", second.WindowSeconds)
	}
	if second.UsedPercent <= 0 || second.UsedPercent > 100 {
		t.Errorf("使用率が %v", second.UsedPercent)
	}
}

// ロードアベレージは単体で意味がある。使用率が出せない 1 回目でも見せる。
func TestLoadAverageIsReportedEvenWhenPercentIsNot(t *testing.T) {
	t.Parallel()

	p := newTestProbe(procStatSample, meminfoSample, "1.5 1.0 0.5 1/2 3", port.StorageUsage{})

	got := p.Read("/tmp").CPU
	if got.Available {
		t.Fatal("1 回目から使用率が出ている")
	}
	if got.Load1 != 1.5 || got.Load5 != 1.0 || got.Load15 != 0.5 {
		t.Errorf("load が %v %v %v", got.Load1, got.Load5, got.Load15)
	}
	if got.Cores <= 0 {
		t.Errorf("コア数が %d", got.Cores)
	}
}

// 1 項目読めないだけで全体を落とさない。資源が苦しいときにこそ開く画面で、
// 何も見えなくなるのが一番困る。
func TestReadDegradesPerItem(t *testing.T) {
	t.Parallel()

	p := newTestProbe(procStatSample, meminfoSample, "0.1 0.2 0.3", port.StorageUsage{})
	p.readMem = func() (string, error) { return "", errors.New("読めません") }
	p.readDisk = func(string) (port.StorageUsage, error) {
		return port.StorageUsage{}, errors.New("容量を取得できません")
	}

	got := p.Read("/srv/mc")

	if got.Memory.Available {
		t.Error("メモリが available になっている")
	}
	if got.Memory.UnavailableReason == "" {
		t.Error("メモリの理由が空")
	}
	if got.Storage.Available {
		t.Error("ストレージが available になっている")
	}
	// どのパスを測ろうとしたかは、失敗したときこそ知りたい。
	if got.Storage.Path != "/srv/mc" {
		t.Errorf("Path が %q", got.Storage.Path)
	}
	// CPU 側は生きている（1 回目なので使用率は出ないが、コア数は出る）。
	if got.CPU.Cores <= 0 {
		t.Errorf("コア数が %d", got.CPU.Cores)
	}
}

// /proc/stat が壊れていても、メモリとストレージは返す。
func TestReadSurvivesBrokenProcStat(t *testing.T) {
	t.Parallel()

	p := newTestProbe("壊れています", meminfoSample, "0.1 0.2 0.3",
		port.StorageUsage{Available: true, TotalBytes: 100, UsedBytes: 40})

	got := p.Read("/tmp")

	if got.CPU.Available {
		t.Error("CPU が available になっている")
	}
	if !got.Memory.Available {
		t.Error("メモリまで落ちている")
	}
	if !got.Storage.Available {
		t.Error("ストレージまで落ちている")
	}
}
