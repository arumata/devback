//go:build darwin

package lock

import (
	"time"

	"golang.org/x/sys/unix"
)

func getProcessStartTimeDarwin(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || info == nil {
		return time.Time{}, false
	}
	tv := info.Proc.P_starttime
	return time.Unix(int64(tv.Sec), int64(tv.Usec)*1000), true
}

func getProcessStartTimeWindows(pid int) (time.Time, bool) {
	_ = pid
	return time.Time{}, false
}
