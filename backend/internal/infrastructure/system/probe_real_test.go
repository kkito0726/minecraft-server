package system

import (
	"runtime"
	"testing"

	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// 実機の読み取り口を 1 度だけ通す。値そのものは機械によって違うので
// 検証しない。見るのは「この OS で読める経路が繋がっているか」だけ。
func TestReadOnThisMachine(t *testing.T) {
	t.Parallel()

	p := New()
	got := p.Read(t.TempDir())

	// 容量はどの unix でも読める。
	assertStorageReadable(t, got.Storage)

	// /proc は Linux だけ。開発機（macOS）では読めないのが正しい。
	if runtime.GOOS == "linux" {
		if !got.Memory.Available {
			t.Errorf("Linux でメモリを読めない: %q", got.Memory.UnavailableReason)
		}
		// CPU 使用率は 2 回目から。1 回目は理由が入っていればよい。
		if !got.CPU.Available && got.CPU.UnavailableReason == "" {
			t.Error("CPU の理由が空")
		}
	} else if got.Memory.Available {
		t.Error("Linux 以外でメモリが読めている")
	}
}

func assertStorageReadable(t *testing.T, s port.StorageUsage) {
	t.Helper()

	if !s.Available {
		t.Fatalf("容量を読めない: %q", s.UnavailableReason)
	}
	if s.TotalBytes <= 0 {
		t.Errorf("TotalBytes が %d", s.TotalBytes)
	}
	// df と同じ Bavail を使うので、使用量と空きの合計は予約ブロックの分だけ
	// 総量に届かない。逆転していないことだけを見る。
	if s.UsedBytes+s.AvailableBytes > s.TotalBytes {
		t.Errorf("使用 %d + 空き %d が総量 %d を超えている",
			s.UsedBytes, s.AvailableBytes, s.TotalBytes)
	}
	if s.UsedPercent < 0 || s.UsedPercent > 100 {
		t.Errorf("UsedPercent が %v", s.UsedPercent)
	}
}
