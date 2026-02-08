//go:build !windows

package process

import (
	"context"
	"syscall"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

// IsProcessRunning checks if a process with given PID is running.
func (a *Adapter) IsProcessRunning(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}

	// Send signal 0 to check if process exists.
	return syscall.Kill(pid, 0) == nil
}

// KillProcess kills a process with given PID.
func (a *Adapter) KillProcess(ctx context.Context, pid int) error {
	if pid <= 0 {
		return syscall.EINVAL
	}

	return syscall.Kill(pid, syscall.SIGTERM)
}

// GetProcessInfo returns process information.
func (a *Adapter) GetProcessInfo(ctx context.Context, pid int) (usecase.ProcessInfo, error) {
	if pid <= 0 {
		return usecase.ProcessInfo{}, syscall.EINVAL
	}

	// Basic process info - in a real implementation, this could be more detailed.
	return usecase.ProcessInfo{
		PID:        pid,
		Name:       "unknown",
		StartTime:  time.Now(),
		CPUPercent: 0.0,
		MemoryMB:   0,
	}, nil
}
