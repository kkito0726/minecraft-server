// Package system はホスト（Pi 本体）の資源の使用状況を読む。
//
// 読む場所は OS によって違うが、解釈はどこでも同じなので、
// 解釈だけをこのファイルに切り出してある。ここにビルドタグは付けない。
// 開発機（macOS）でも /proc の断片を文字列として渡せばテストできる。
package system

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// cpuSample は /proc/stat の 1 点。単位は jiffies。
type cpuSample struct {
	// total は全ての欄の合計。
	total float64
	// idle は idle と iowait の合計。「働いていない」側。
	idle float64
	at   time.Time
}

// parseProcStat は /proc/stat の先頭の cpu 行を読む。
//
// 欄は user nice system idle iowait irq softirq steal guest guest_nice の順。
// 増えることはあっても減らないので、位置で見るのは先頭 5 つまでにして、
// 残りは合計に足すだけにする。カーネルが欄を増やしても壊れない。
func parseProcStat(s string, at time.Time) (cpuSample, error) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 5 {
			return cpuSample{}, fmt.Errorf("cpu 行の欄が足りません: %q", line)
		}

		sample := cpuSample{at: at}
		for i, f := range fields[1:] {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return cpuSample{}, fmt.Errorf("cpu 行を解釈できません: %w", err)
			}
			sample.total += v
			// 4 番目が idle、5 番目が iowait（fields[1:] では 3 と 4）。
			if i == 3 || i == 4 {
				sample.idle += v
			}
		}
		return sample, nil
	}
	return cpuSample{}, fmt.Errorf("cpu 行が見つかりません")
}

// cpuPercent は 2 点の差分から使用率を求める。
//
// 累積値どうしの引き算なので、間に再起動を挟むと負になりうる。
// その場合は「測れなかった」として扱い、誤った数字を出さない。
func cpuPercent(prev, cur cpuSample) (percent, window float64, ok bool) {
	dTotal := cur.total - prev.total
	dIdle := cur.idle - prev.idle
	if dTotal <= 0 || dIdle < 0 {
		return 0, 0, false
	}

	percent = (dTotal - dIdle) / dTotal * 100
	// 丸め誤差で 100 をわずかに超えることがある。画面に 100.3% と
	// 出ると読み手が値そのものを疑うので、ここで挟む。
	percent = min(max(percent, 0), 100)

	return percent, cur.at.Sub(prev.at).Seconds(), true
}

// parseMeminfo は /proc/meminfo を読む。単位は kB。
//
// MemFree ではなく MemAvailable を使う。MemFree はページキャッシュを
// 「使用中」に数えるため、実用上は余っているのに逼迫して見える。
func parseMeminfo(s string) (port.MemoryUsage, error) {
	values := map[string]int64{}
	for _, line := range strings.Split(s, "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[key] = v * 1024
	}

	total, ok := values["MemTotal"]
	if !ok || total <= 0 {
		return port.MemoryUsage{}, fmt.Errorf("MemTotal を読めません")
	}
	available, ok := values["MemAvailable"]
	if !ok {
		return port.MemoryUsage{}, fmt.Errorf("MemAvailable を読めません")
	}

	used := total - available
	swapTotal := values["SwapTotal"]

	return port.MemoryUsage{
		Available:      true,
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsedPercent:    float64(used) / float64(total) * 100,
		SwapTotalBytes: swapTotal,
		SwapUsedBytes:  swapTotal - values["SwapFree"],
	}, nil
}

// parseLoadavg は /proc/loadavg の先頭 3 つを読む。
func parseLoadavg(s string) (load1, load5, load15 float64, err error) {
	fields := strings.Fields(s)
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("loadavg の欄が足りません: %q", s)
	}

	out := make([]float64, 3)
	for i := range out {
		v, parseErr := strconv.ParseFloat(fields[i], 64)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("loadavg を解釈できません: %w", parseErr)
		}
		out[i] = v
	}
	return out[0], out[1], out[2], nil
}
