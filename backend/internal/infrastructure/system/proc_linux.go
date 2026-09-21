//go:build linux

package system

import "os"

// /proc を読むのは Linux だけ。開発機（macOS）では proc_other.go 側が
// 「読めない」を返す。ビルドを通らなくするのではなく読めないで倒すのは、
// 開発機でも画面そのものは開けたほうが確かめやすいため。

func readProcStat() (string, error)    { return readFile("/proc/stat") }
func readProcMeminfo() (string, error) { return readFile("/proc/meminfo") }
func readProcLoadavg() (string, error) { return readFile("/proc/loadavg") }

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // 読む先は固定の /proc
	if err != nil {
		return "", err
	}
	return string(b), nil
}
