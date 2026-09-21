package rpc

import (
	"context"

	"connectrpc.com/connect"

	mcadminv1 "github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1"
	"github.com/kkito0726/minecraft-server/backend/gen/mcadmin/v1/mcadminv1connect"
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/sysmetrics"
)

// SystemHandler は SystemService のハンドラ。
type SystemHandler struct {
	mcadminv1connect.UnimplementedSystemServiceHandler

	metrics *sysmetrics.UseCase
}

// NewSystemHandler は SystemHandler を作る。
func NewSystemHandler(metrics *sysmetrics.UseCase) *SystemHandler {
	return &SystemHandler{metrics: metrics}
}

// GetMetrics はホストの資源の使用状況を返す。
//
// エラーを返す経路が無い。読めなかった項目は available が偽で返る。
func (h *SystemHandler) GetMetrics(
	_ context.Context,
	_ *connect.Request[mcadminv1.GetMetricsRequest],
) (*connect.Response[mcadminv1.GetMetricsResponse], error) {
	m := h.metrics.Execute()

	return connect.NewResponse(&mcadminv1.GetMetricsResponse{
		Cpu:     cpuUsageToProto(m.CPU),
		Memory:  memoryUsageToProto(m.Memory),
		Storage: storageUsageToProto(m.Storage),
	}), nil
}

func cpuUsageToProto(u port.CPUUsage) *mcadminv1.CpuUsage {
	return &mcadminv1.CpuUsage{
		Available:         u.Available,
		UsedPercent:       u.UsedPercent,
		WindowSeconds:     u.WindowSeconds,
		Cores:             int32(u.Cores), //nolint:gosec // コア数が int32 を超えることはない
		Load1:             u.Load1,
		Load5:             u.Load5,
		Load15:            u.Load15,
		UnavailableReason: u.UnavailableReason,
	}
}

func memoryUsageToProto(u port.MemoryUsage) *mcadminv1.MemoryUsage {
	return &mcadminv1.MemoryUsage{
		Available:         u.Available,
		TotalBytes:        u.TotalBytes,
		AvailableBytes:    u.AvailableBytes,
		UsedBytes:         u.UsedBytes,
		UsedPercent:       u.UsedPercent,
		SwapTotalBytes:    u.SwapTotalBytes,
		SwapUsedBytes:     u.SwapUsedBytes,
		UnavailableReason: u.UnavailableReason,
	}
}

func storageUsageToProto(u port.StorageUsage) *mcadminv1.StorageUsage {
	return &mcadminv1.StorageUsage{
		Available:         u.Available,
		Path:              u.Path,
		TotalBytes:        u.TotalBytes,
		AvailableBytes:    u.AvailableBytes,
		UsedBytes:         u.UsedBytes,
		UsedPercent:       u.UsedPercent,
		UnavailableReason: u.UnavailableReason,
	}
}
