//go:build windows

package lock

import "golang.org/x/sys/windows"

// isProcessRunning checks if process with given PID is running.
func (a *Adapter) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windows.STILL_ACTIVE
}
