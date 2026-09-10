// Package server はサーバーの状態を表すドメインモデル。
package server

import "time"

// ContainerState はコンテナの状態。
type ContainerState int

const (
	// ContainerMissing はコンテナが存在しないことを表す（down 済み）。
	ContainerMissing ContainerState = iota
	// ContainerRunning は実行中。
	ContainerRunning
	// ContainerExited は終了している。
	ContainerExited
	// ContainerRestarting は再起動中。
	ContainerRestarting
)

// String は状態の名前を返す。
func (s ContainerState) String() string {
	switch s {
	case ContainerRunning:
		return "実行中"
	case ContainerExited:
		return "停止"
	case ContainerRestarting:
		return "再起動中"
	case ContainerMissing:
		return "コンテナなし"
	default:
		return "不明"
	}
}

// IsUp はコンテナが動いているかを返す。
// RCON を伴う手順を行ってよいかの判断に使う。
func (s ContainerState) IsUp() bool { return s == ContainerRunning }

// IsStopped は停止しきっているかを返す。
//
// docker compose down の完了待ちに使う。down はコンテナを削除するので
// 通常は ContainerMissing になるが、stop だけされた状態も停止として扱う。
// 再起動中は「これから動く」ので停止とみなさない。
func (s ContainerState) IsStopped() bool {
	return s == ContainerMissing || s == ContainerExited
}

// ContainerStatus はコンテナの状態一式。
type ContainerStatus struct {
	State     ContainerState
	Healthy   bool
	StartedAt time.Time
	Image     string
}

// SavingState はワールドの保存が有効かどうかの推定値。
//
// RCON には保存状態を問い合わせる手段がないため実測できない。
// save-on が冪等であることを利用して無条件に再送する設計であり、
// ここに出るのはあくまで推定である。
type SavingState int

const (
	// SavingAssumedOn は保存が有効と推定されることを表す。
	SavingAssumedOn SavingState = iota
	// SavingSuspectOff は中断された操作があり、save-off が残っている
	// 可能性があることを表す。
	SavingSuspectOff
)

// String は状態の名前を返す。
func (s SavingState) String() string {
	switch s {
	case SavingSuspectOff:
		return "停止している可能性あり"
	default:
		return "有効（推定）"
	}
}
