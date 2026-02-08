//go:build windows

package process

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/arumata/devback/internal/usecase"
)

// IsProcessRunning checks if a process with given PID is running.
func (a *Adapter) IsProcessRunning(ctx context.Context, pid int) bool {
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

// KillProcess kills a process with given PID.
func (a *Adapter) KillProcess(ctx context.Context, pid int) error {
	if pid <= 0 {
		return syscall.EINVAL
	}

	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process: %w", err)
	}
	defer windows.CloseHandle(handle)

	if err := windows.TerminateProcess(handle, 1); err != nil {
		return fmt.Errorf("terminate process: %w", err)
	}
	return nil
}

// GetProcessInfo returns process information.
func (a *Adapter) GetProcessInfo(ctx context.Context, pid int) (usecase.ProcessInfo, error) {
	if pid <= 0 {
		return usecase.ProcessInfo{}, syscall.EINVAL
	}

	return usecase.ProcessInfo{
		PID:        pid,
		Name:       "unknown",
		StartTime:  time.Now(),
		CPUPercent: 0.0,
		MemoryMB:   0,
	}, nil
}
