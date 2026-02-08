//nolint:gci,gofumpt
package lock

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

const (
	osLinux  = "linux"
	osDarwin = "darwin"
)

// Adapter implements LockAdapter using filesystem-based locking
type Adapter struct {
	logger *slog.Logger
}

// New creates a new lock adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("lock adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// AcquireLock attempts to acquire exclusive lock
func (a *Adapter) AcquireLock(ctx context.Context, path string, info usecase.LockInfo) error {
	// First try to create lock directory (backward compatibility)
	lockDir := path
	if err := os.Mkdir(lockDir, 0o750); err == nil {
		// Successfully created directory, now create info file inside
		lockFile := filepath.Join(lockDir, "info")
		return a.createLockFile(lockFile, info)
	} else if !os.IsExist(err) {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Directory already exists, check info file
	lockFile := filepath.Join(lockDir, "info")
	isValid := a.validateLockFile(lockFile, 24*time.Hour)

	if !isValid {
		// Lock is invalid, try to remove and create new
		if err := os.RemoveAll(lockDir); err != nil {
			return fmt.Errorf("failed to remove stale lock: %w", err)
		}

		if err := os.Mkdir(lockDir, 0o750); err != nil {
			return fmt.Errorf("failed to create lock after cleanup: %w", err)
		}

		lockFile = filepath.Join(lockDir, "info")
		return a.createLockFile(lockFile, info)
	}

	// Lock is valid, cannot acquire
	return fmt.Errorf("lock is held by another active process")
}

// ReleaseLock releases held lock
func (a *Adapter) ReleaseLock(ctx context.Context, path string) error {
	return os.RemoveAll(path)
}

// IsLocked checks if path is locked
func (a *Adapter) IsLocked(ctx context.Context, path string) (bool, usecase.LockInfo, error) {
	lockFile := filepath.Join(path, "info")

	// Check if lock directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, usecase.LockInfo{}, nil
	}

	// Check if info file exists
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		return false, usecase.LockInfo{}, nil
	}

	// Read and validate lock info
	info, err := a.readLockInfo(lockFile)
	if err != nil {
		return false, usecase.LockInfo{}, err
	}

	// Validate if lock is still active
	valid := a.validateLockFile(lockFile, 24*time.Hour)

	return valid, info, nil
}

// RefreshLock updates lock timestamp
func (a *Adapter) RefreshLock(ctx context.Context, path string) error {
	lockFile := filepath.Join(path, "info")

	// Read current lock info
	info, err := a.readLockInfo(lockFile)
	if err != nil {
		return fmt.Errorf("failed to read lock info: %w", err)
	}

	// Update start time to current time
	info.StartTime = time.Now()

	// Write updated info back
	return a.createLockFile(lockFile, info)
}

// CleanupStaleLocks removes stale locks
func (a *Adapter) CleanupStaleLocks(ctx context.Context, maxAge time.Duration) error {
	// This is a simplified implementation
	// In a real scenario, we would need to scan known lock directories
	// For now, return nil as this would typically be called by a background process
	return nil
}

// createLockFile creates lock file with process information
func (a *Adapter) createLockFile(lockPath string, info usecase.LockInfo) error {
	// Fill in missing information if not provided
	if info.PID == 0 {
		info.PID = os.Getpid()
	}
	if info.StartTime.IsZero() {
		info.StartTime = time.Now()
	}
	if info.Hostname == "" {
		hostname, _ := os.Hostname()
		info.Hostname = hostname
	}
	if info.ProcessStartTicks == 0 {
		if ticks, ok := getProcessStartTicks(info.PID); ok {
			info.ProcessStartTicks = ticks
		}
	}
	if info.ProcessStartID == "" {
		if id, ok := getProcessStartID(info.PID); ok {
			info.ProcessStartID = id
		}
	}

	// Write as JSON for better structure
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal lock info: %w", err)
	}

	return os.WriteFile(lockPath, data, 0o600)
}

// readLockInfo reads lock information from file
func (a *Adapter) readLockInfo(lockPath string) (usecase.LockInfo, error) {
	data, err := os.ReadFile(lockPath) // #nosec G304 - lockPath is controlled by the adapter
	if err != nil {
		return usecase.LockInfo{}, err
	}

	var info usecase.LockInfo

	// Try JSON format first (new format)
	if err := json.Unmarshal(data, &info); err == nil {
		return info, nil
	}

	// Fallback to legacy format (PID\nTimestamp\nHostname)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return usecase.LockInfo{}, fmt.Errorf("invalid lock file format")
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return usecase.LockInfo{}, fmt.Errorf("invalid PID in lock file: %w", err)
	}

	startTimeUnix, err := strconv.ParseInt(lines[1], 10, 64)
	if err != nil {
		return usecase.LockInfo{}, fmt.Errorf("invalid timestamp in lock file: %w", err)
	}

	hostname := ""
	if len(lines) > 2 {
		hostname = lines[2]
	}

	info = usecase.LockInfo{
		PID:       pid,
		StartTime: time.Unix(startTimeUnix, 0),
		Hostname:  hostname,
	}

	return info, nil
}

// validateLockFile checks if lock file is valid and process is still running
func (a *Adapter) validateLockFile(lockPath string, maxAge time.Duration) bool {
	info, err := a.readLockInfo(lockPath)
	if err != nil {
		return false // Invalid file format means invalid lock
	}

	// Check age
	if time.Since(info.StartTime) > maxAge {
		return false // Lock is too old
	}

	if info.Hostname != "" {
		if hostname, err := os.Hostname(); err == nil && hostname != info.Hostname {
			return true
		}
	}

	if info.ProcessStartID != "" {
		if id, ok := getProcessStartID(info.PID); ok {
			return id == info.ProcessStartID
		}
	}

	if info.ProcessStartTicks != 0 {
		if ticks, ok := getProcessStartTicks(info.PID); ok {
			if ticks != info.ProcessStartTicks {
				return false // PID reused
			}
			return true
		}
	}

	// Check if process is still running (fallback)
	if !a.isProcessRunning(info.PID) {
		return false // Process is not running
	}

	return true
}

func getProcessStartTicks(pid int) (int64, bool) {
	if pid <= 0 {
		return 0, false
	}
	if runtime.GOOS != osLinux {
		return 0, false
	}
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	// #nosec G304 -- reading /proc/<pid>/stat from controlled path.
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, false
	}
	parts := strings.Fields(string(data))
	if len(parts) < 22 {
		return 0, false
	}
	startTicks, err := strconv.ParseInt(parts[21], 10, 64)
	if err != nil {
		return 0, false
	}
	return startTicks, true
}

func getProcessStartID(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	switch runtime.GOOS {
	case osLinux:
		if ticks, ok := getProcessStartTicks(pid); ok {
			return fmt.Sprintf("ticks:%d", ticks), true
		}
		return "", false
	case osDarwin:
		startTime, ok := getProcessStartTimeDarwin(pid)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("lstart:%d", startTime.UnixNano()), true
	case "windows":
		startTime, ok := getProcessStartTimeWindows(pid)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("ctime:%d", startTime.UnixNano()), true
	default:
		return "", false
	}
}
