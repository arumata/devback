//go:build !darwin && !windows

package lock

import "time"

func getProcessStartTimeDarwin(pid int) (time.Time, bool) {
	_ = pid
	return time.Time{}, false
}

func getProcessStartTimeWindows(pid int) (time.Time, bool) {
	_ = pid
	return time.Time{}, false
}
