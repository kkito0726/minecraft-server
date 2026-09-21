package system

import (
	"math"
	"testing"
	"time"
)

// 実機の /proc/stat の先頭。欄は user nice system idle iowait ... と続く。
const procStatSample = `cpu  10 20 30 400 50 6 7 0 0 0
cpu0 1 2 3 40 5 0 0 0 0 0
intr 12345
`

func TestParseProcStat(t *testing.T) {
	t.Parallel()

	at := time.Unix(1000, 0)
	got, err := parseProcStat(procStatSample, at)
	if err != nil {
		t.Fatalf("解釈に失敗: %v", err)
	}

	// 10+20+30+400+50+6+7 = 523
	if got.total != 523 {
		t.Errorf("total が %v", got.total)
	}
	// idle(400) + iowait(50) = 450
	if got.idle != 450 {
		t.Errorf("idle が %v", got.idle)
	}
	if !got.at.Equal(at) {
		t.Errorf("at が %v", got.at)
	}
}

// カーネルが欄を増やしても壊れないこと。
// 位置で見るのを先頭 5 つに限っているのはこのため。
func TestParseProcStatWithUnknownColumns(t *testing.T) {
	t.Parallel()

	got, err := parseProcStat("cpu  1 1 1 1 1 1 1 1 1 1 1 1\n", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("解釈に失敗: %v", err)
	}
	if got.total != 12 {
		t.Errorf("total が %v", got.total)
	}
	if got.idle != 2 {
		t.Errorf("idle が %v", got.idle)
	}
}

func TestParseProcStatErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"cpu 行が無い": "intr 1\nctxt 2\n",
		"欄が足りない":   "cpu  1 2 3\n",
		"数として読めない": "cpu  1 2 3 x 5\n",
		"そもそも空":    "",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseProcStat(input, time.Unix(0, 0)); err == nil {
				t.Error("エラーを期待したが nil")
			}
		})
	}
}

func TestCPUPercent(t *testing.T) {
	t.Parallel()

	prev := cpuSample{total: 1000, idle: 800, at: time.Unix(0, 0)}
	cur := cpuSample{total: 1100, idle: 860, at: time.Unix(5, 0)}

	// 区間の合計 100 のうち idle が 60。使用率は 40%。
	percent, window, ok := cpuPercent(prev, cur)
	if !ok {
		t.Fatal("測れないと判定された")
	}
	if math.Abs(percent-40) > 1e-9 {
		t.Errorf("使用率が %v", percent)
	}
	if window != 5 {
		t.Errorf("区間が %v 秒", window)
	}
}

// 再起動を挟むと累積値が巻き戻る。負の使用率を出すより、
// 測れなかったことにする。
func TestCPUPercentRejectsInvalidDeltas(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ prev, cur cpuSample }{
		"差が無い": {
			prev: cpuSample{total: 100, idle: 50},
			cur:  cpuSample{total: 100, idle: 50},
		},
		"巻き戻っている": {
			prev: cpuSample{total: 1000, idle: 800},
			cur:  cpuSample{total: 100, idle: 80},
		},
		"idle だけ巻き戻っている": {
			prev: cpuSample{total: 1000, idle: 900},
			cur:  cpuSample{total: 1100, idle: 800},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, ok := cpuPercent(tt.prev, tt.cur); ok {
				t.Error("測れたと判定された")
			}
		})
	}
}

// 丸め誤差で 100 を超えた値を画面に出さない。
func TestCPUPercentClamps(t *testing.T) {
	t.Parallel()

	prev := cpuSample{total: 0, idle: 0}
	cur := cpuSample{total: 100, idle: 0}

	percent, _, ok := cpuPercent(prev, cur)
	if !ok {
		t.Fatal("測れないと判定された")
	}
	if percent != 100 {
		t.Errorf("使用率が %v", percent)
	}
}

const meminfoSample = `MemTotal:        4194304 kB
MemFree:          262144 kB
MemAvailable:    1048576 kB
Buffers:           65536 kB
SwapTotal:        102400 kB
SwapFree:          51200 kB
`

func TestParseMeminfo(t *testing.T) {
	t.Parallel()

	got, err := parseMeminfo(meminfoSample)
	if err != nil {
		t.Fatalf("解釈に失敗: %v", err)
	}
	if !got.Available {
		t.Error("Available が偽")
	}
	if got.TotalBytes != 4194304*1024 {
		t.Errorf("TotalBytes が %d", got.TotalBytes)
	}
	// MemFree(262144) ではなく MemAvailable(1048576) を使う。
	if got.AvailableBytes != 1048576*1024 {
		t.Errorf("AvailableBytes が %d", got.AvailableBytes)
	}
	if got.UsedBytes != (4194304-1048576)*1024 {
		t.Errorf("UsedBytes が %d", got.UsedBytes)
	}
	if math.Abs(got.UsedPercent-75) > 1e-9 {
		t.Errorf("UsedPercent が %v", got.UsedPercent)
	}
	if got.SwapUsedBytes != 51200*1024 {
		t.Errorf("SwapUsedBytes が %d", got.SwapUsedBytes)
	}
}

func TestParseMeminfoErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"MemTotal が無い":     "MemAvailable: 100 kB\n",
		"MemAvailable が無い": "MemTotal: 100 kB\n",
		"MemTotal が 0":     "MemTotal: 0 kB\nMemAvailable: 0 kB\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseMeminfo(input); err == nil {
				t.Error("エラーを期待したが nil")
			}
		})
	}
}

// スワップを無効にした Pi では SwapTotal が 0 で、SwapFree の行が無い。
func TestParseMeminfoWithoutSwap(t *testing.T) {
	t.Parallel()

	got, err := parseMeminfo("MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	if err != nil {
		t.Fatalf("解釈に失敗: %v", err)
	}
	if got.SwapTotalBytes != 0 || got.SwapUsedBytes != 0 {
		t.Errorf("スワップが %d / %d", got.SwapTotalBytes, got.SwapUsedBytes)
	}
}

func TestParseLoadavg(t *testing.T) {
	t.Parallel()

	l1, l5, l15, err := parseLoadavg("0.52 0.48 0.35 1/234 5678\n")
	if err != nil {
		t.Fatalf("解釈に失敗: %v", err)
	}
	if l1 != 0.52 || l5 != 0.48 || l15 != 0.35 {
		t.Errorf("load が %v %v %v", l1, l5, l15)
	}
}

func TestParseLoadavgErrors(t *testing.T) {
	t.Parallel()

	for name, input := range map[string]string{
		"欄が足りない":   "0.5 0.4\n",
		"数として読めない": "a b c\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, _, err := parseLoadavg(input); err == nil {
				t.Error("エラーを期待したが nil")
			}
		})
	}
}
