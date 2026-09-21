package rpc_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/sysmetrics"
	"github.com/kkito0726/minecraft-server/backend/internal/presentation/rpc"
)

// fakeProbe は渡された値をそのまま返す。測り方そのものは
// infrastructure/system のテストで見ているので、ここでは詰め替えだけを見る。
type fakeProbe struct {
	metrics port.SystemMetrics
	gotPath string
}

func (f *fakeProbe) Read(path string) port.SystemMetrics {
	f.gotPath = path
	return f.metrics
}

func TestGetMetrics(t *testing.T) {
	t.Parallel()

	probe := &fakeProbe{metrics: port.SystemMetrics{
		CPU: port.CPUUsage{
			Available: true, UsedPercent: 42.5, WindowSeconds: 5, Cores: 4,
			Load1: 1.2, Load5: 0.9, Load15: 0.7,
		},
		Memory: port.MemoryUsage{
			Available: true, TotalBytes: 4 << 30, AvailableBytes: 1 << 30,
			UsedBytes: 3 << 30, UsedPercent: 75,
		},
		Storage: port.StorageUsage{
			Available: true, Path: "/srv/mc", TotalBytes: 100, UsedBytes: 40,
			AvailableBytes: 55, UsedPercent: 40,
		},
	}}
	h := rpc.NewSystemHandler(sysmetrics.NewUseCase(probe, "/srv/mc"))

	res, err := h.GetMetrics(context.Background(),
		connect.NewRequest(&mcadminv1.GetMetricsRequest{}))
	if err != nil {
		t.Fatalf("GetMetrics に失敗: %v", err)
	}

	if probe.gotPath != "/srv/mc" {
		t.Errorf("測った先が %q", probe.gotPath)
	}

	cpu := res.Msg.GetCpu()
	if !cpu.GetAvailable() || cpu.GetUsedPercent() != 42.5 || cpu.GetCores() != 4 {
		t.Errorf("CPU が %+v", cpu)
	}
	if cpu.GetWindowSeconds() != 5 {
		t.Errorf("区間が %v", cpu.GetWindowSeconds())
	}
	if res.Msg.GetMemory().GetUsedPercent() != 75 {
		t.Errorf("メモリが %+v", res.Msg.GetMemory())
	}
	if res.Msg.GetStorage().GetPath() != "/srv/mc" {
		t.Errorf("ストレージが %+v", res.Msg.GetStorage())
	}
}

// 読めなかった項目は available が偽で理由付きで返る。
// ここでエラーにすると、資源が苦しいときに画面ごと開けなくなる。
func TestGetMetricsDoesNotFailWhenUnavailable(t *testing.T) {
	t.Parallel()

	probe := &fakeProbe{metrics: port.SystemMetrics{
		CPU:     port.CPUUsage{UnavailableReason: "測定中です（次の取得から出ます）", Cores: 4},
		Memory:  port.MemoryUsage{UnavailableReason: "この OS では取得できません"},
		Storage: port.StorageUsage{Path: "/srv/mc", UnavailableReason: "容量を取得できません"},
	}}
	h := rpc.NewSystemHandler(sysmetrics.NewUseCase(probe, "/srv/mc"))

	res, err := h.GetMetrics(context.Background(),
		connect.NewRequest(&mcadminv1.GetMetricsRequest{}))
	if err != nil {
		t.Fatalf("エラーを返した: %v", err)
	}

	if res.Msg.GetCpu().GetAvailable() || res.Msg.GetMemory().GetAvailable() {
		t.Error("available が真になっている")
	}
	if res.Msg.GetCpu().GetUnavailableReason() == "" {
		t.Error("CPU の理由が空")
	}
	// 使用率が出せなくてもコア数は返る。
	if res.Msg.GetCpu().GetCores() != 4 {
		t.Errorf("コア数が %d", res.Msg.GetCpu().GetCores())
	}
}
