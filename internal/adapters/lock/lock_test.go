package lock

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

func TestAdapter_LockLifecycle(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")

	info := usecase.LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now(),
		RepoPath:  "/repo",
		BackupDir: "/backup",
	}

	if err := adapter.AcquireLock(ctx, lockPath, info); err != nil {
		t.Fatal(err)
	}

	locked, got, err := adapter.IsLocked(ctx, lockPath)
	if err != nil || !locked {
		t.Fatal("expected lock to be active")
	}
	if got.PID == 0 {
		t.Fatal("expected pid in lock info")
	}

	if err := adapter.RefreshLock(ctx, lockPath); err != nil {
		t.Fatal(err)
	}

	if err := adapter.ReleaseLock(ctx, lockPath); err != nil {
		t.Fatal(err)
	}

	locked, _, err = adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be released")
	}
}

func TestAdapter_AcquireLockConflict(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")

	info := usecase.LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now(),
		RepoPath:  "/repo",
		BackupDir: "/backup",
	}

	if err := adapter.AcquireLock(ctx, lockPath, info); err != nil {
		t.Fatal(err)
	}

	if err := adapter.AcquireLock(ctx, lockPath, info); err == nil {
		t.Fatal("expected lock conflict")
	}
}

func TestAdapter_StaleLock(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	stale := usecase.LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now().Add(-48 * time.Hour),
		RepoPath:  "/repo",
		BackupDir: "/backup",
		Hostname:  mustHostname(t),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected stale lock to be inactive")
	}
}

func TestAdapter_LegacyLockFormat(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	data := []byte(
		fmt.Sprintf("%d\n%d\n%s",
			os.Getpid(),
			time.Now().Unix(),
			mustHostname(t),
		),
	)
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	locked, info, err := adapter.IsLocked(ctx, lockPath)
	if err != nil || !locked {
		t.Fatal("expected legacy lock to be active")
	}
	if info.PID == 0 {
		t.Fatal("expected pid in lock info")
	}
}

func TestAdapter_AcquireStaleLock(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	stale := usecase.LockInfo{
		PID:       0,
		StartTime: time.Now().Add(-48 * time.Hour),
		RepoPath:  "/repo",
		BackupDir: "/backup",
		Hostname:  mustHostname(t),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := adapter.AcquireLock(ctx, lockPath, usecase.LockInfo{PID: os.Getpid(), StartTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestAdapter_IsLocked_NoLock(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be inactive")
	}
}

func TestAdapter_CleanupStaleLocks(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	if err := adapter.CleanupStaleLocks(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
}

func TestAdapter_AcquireLock_FillsDefaults(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")

	if err := adapter.AcquireLock(ctx, lockPath, usecase.LockInfo{}); err != nil {
		t.Fatal(err)
	}

	locked, info, err := adapter.IsLocked(ctx, lockPath)
	if err != nil || !locked {
		t.Fatal("expected lock to be active")
	}
	if info.PID == 0 || info.Hostname == "" {
		t.Fatal("expected lock info defaults to be filled")
	}
	if (runtime.GOOS == osLinux || runtime.GOOS == osDarwin) && info.ProcessStartID == "" {
		t.Fatal("expected process start id to be set")
	}
}

func TestAdapter_IsLocked_ProcessStartMismatch(t *testing.T) {
	if runtime.GOOS != osLinux && runtime.GOOS != osDarwin {
		t.Skip("process start id validation is only available on linux/darwin")
	}
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	info := usecase.LockInfo{
		PID:            os.Getpid(),
		StartTime:      time.Now(),
		Hostname:       mustHostname(t),
		ProcessStartID: "mismatch",
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be inactive with mismatched process start ticks")
	}
}

func TestAdapter_IsLocked_InvalidPID(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	invalid := usecase.LockInfo{
		PID:       0,
		StartTime: time.Now(),
		Hostname:  mustHostname(t),
	}
	data, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be inactive for invalid pid")
	}
}

func TestAdapter_IsLocked_InvalidProcess(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")
	lockFile := filepath.Join(lockPath, "info")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	data := []byte(
		fmt.Sprintf("%d\n%d\n%s",
			999999,
			time.Now().Unix(),
			mustHostname(t),
		),
	)
	if err := os.WriteFile(lockFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be inactive for missing process")
	}
}

func TestAdapter_AcquireLock_InvalidPath(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	lockPath := filepath.Join(t.TempDir(), "missing", ".lock")

	if err := adapter.AcquireLock(ctx, lockPath, usecase.LockInfo{PID: os.Getpid(), StartTime: time.Now()}); err == nil {
		t.Fatal("expected error for invalid lock path")
	}
}

func TestAdapter_IsLocked_NoInfoFile(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, ".lock")

	if err := os.MkdirAll(lockPath, 0o750); err != nil {
		t.Fatal(err)
	}

	locked, _, err := adapter.IsLocked(ctx, lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected lock to be inactive without info file")
	}
}

func mustHostname(t *testing.T) string {
	t.Helper()
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("hostname error: %v", err)
	}
	return hostname
}
