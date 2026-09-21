//go:build !linux

package system

import "errors"

// Linux 以外では /proc が無い。開発機で画面を開いたときに
// 「この OS では取得できません」と出す。

var errNoProc = errors.New("この OS では取得できません（/proc がありません）")

func readProcStat() (string, error)    { return "", errNoProc }
func readProcMeminfo() (string, error) { return "", errNoProc }
func readProcLoadavg() (string, error) { return "", errNoProc }
