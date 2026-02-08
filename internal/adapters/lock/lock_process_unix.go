//go:build !windows

package lock

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// isProcessRunning checks if process with given PID is running.
func (a *Adapter) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Check existence through /proc on Linux (and on some other Unix-like systems).
	if runtime.GOOS == osLinux || runtime.GOOS == osDarwin {
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			return true
		}
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 (test signal).
	return process.Signal(syscall.Signal(0)) == nil
}
