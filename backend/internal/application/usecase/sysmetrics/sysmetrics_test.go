package sysmetrics_test

import (
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
	"github.com/kkito0726/minecraft-server/backend/internal/application/usecase/sysmetrics"
)

type fakeProbe struct {
	gotPath string
}

func (f *fakeProbe) Read(path string) port.SystemMetrics {
	f.gotPath = path
	return port.SystemMetrics{CPU: port.CPUUsage{Available: true, UsedPercent: 12.5}}
}

// 測る対象はプロジェクトディレクトリ。ここが data/ と backups/ の載って
// いる場所で、埋まると操作が止まる。別の場所を測ると意味が無い。
func TestExecuteMeasuresProjectDir(t *testing.T) {
	t.Parallel()

	probe := &fakeProbe{}
	got := sysmetrics.NewUseCase(probe, "/home/pi/minecraft-server").Execute()

	if probe.gotPath != "/home/pi/minecraft-server" {
		t.Errorf("測った先が %q", probe.gotPath)
	}
	if got.CPU.UsedPercent != 12.5 {
		t.Errorf("値が素通しされていない: %+v", got.CPU)
	}
}
