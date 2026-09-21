package port

// ホスト（Pi 本体）の資源の使用状況を読むための境界。
//
// エラーを返さないのは意図したもの。この画面は資源が苦しいときにこそ
// 開かれるので、1 項目読めないだけで全体を失敗させると、知りたい時に
// 何も見えなくなる。読めなかった項目は Available を偽にし、理由を
// UnavailableReason に入れて返す。呼ぶ側は分岐を書かずに素通しできる。

// CPUUsage は CPU の使用状況。
type CPUUsage struct {
	Available bool
	// UsedPercent は 0.0〜100.0。全論理コアをまとめた値。
	UsedPercent float64
	// WindowSeconds は差分を取った区間の長さ。
	//
	// /proc/stat は起動からの累積値なので、1 点では「電源を入れてからの
	// 平均」にしかならない。どの期間の平均かは画面にも出す。
	WindowSeconds float64
	Cores         int
	Load1         float64
	Load5         float64
	Load15        float64

	UnavailableReason string
}

// MemoryUsage はメモリの使用状況。
type MemoryUsage struct {
	Available  bool
	TotalBytes int64
	// AvailableBytes は MemAvailable。MemFree ではない。
	// 回収できるキャッシュを含む「実際に使える量」。
	AvailableBytes int64
	UsedBytes      int64
	UsedPercent    float64
	SwapTotalBytes int64
	SwapUsedBytes  int64

	UnavailableReason string
}

// StorageUsage はファイルシステムの使用状況。
type StorageUsage struct {
	Available  bool
	Path       string
	TotalBytes int64
	// AvailableBytes は statfs の Bavail。Bfree ではない。
	// 予約ブロックを除いた、一般ユーザーが実際に使える量。
	AvailableBytes int64
	UsedBytes      int64
	UsedPercent    float64

	UnavailableReason string
}

// SystemMetrics は 3 つをまとめたもの。
type SystemMetrics struct {
	CPU     CPUUsage
	Memory  MemoryUsage
	Storage StorageUsage
}

// SystemProbe はホストの資源を読む。
//
// CPU は差分が要るため、実装は前回の値を保持する。つまり呼ぶ間隔が
// そのまま測定区間になる。
type SystemProbe interface {
	// Read は path が載っているファイルシステムを対象に読む。
	Read(path string) SystemMetrics
}
