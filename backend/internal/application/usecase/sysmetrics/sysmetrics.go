// Package sysmetrics はホスト（Pi 本体）の資源の使用状況を返す。
//
// 対象をコンテナではなくホストにしているのは、ストレージがコンテナと
// 共有であること、そして 4GB の Pi では MC 以外の使用分も含めた全体が
// 分からないと判断できないため。
package sysmetrics

import (
	"github.com/kkito0726/minecraft-server/backend/internal/application/port"
)

// UseCase はホストの資源を読む。
type UseCase struct {
	probe port.SystemProbe
	// path は容量を測る対象。data/ と backups/ が載っている
	// プロジェクトディレクトリを渡す。ここが埋まると操作が止まる。
	path string
}

// NewUseCase は UseCase を作る。
func NewUseCase(probe port.SystemProbe, path string) *UseCase {
	return &UseCase{probe: probe, path: path}
}

// Execute は現在の使用状況を返す。
//
// エラーを返さない。読めなかった項目は Available が偽になって返るだけで、
// 画面は残りを表示できる。資源が苦しいときにこそ開く画面なので、
// 一部が読めないことを全体の失敗にしない。
func (u *UseCase) Execute() port.SystemMetrics {
	return u.probe.Read(u.path)
}
