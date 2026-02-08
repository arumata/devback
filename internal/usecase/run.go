//nolint:gci,gofumpt
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

//nolint:gochecknoglobals // configurable in tests to speed up lock refresh.
var lockRefreshInterval = time.Hour

// TestLocks validates lock behavior using the configured adapters.
func TestLocks(ctx context.Context, cfg *Config, deps *Dependencies, logger *slog.Logger) error {
	if logger == nil {
		panic("logger is required")
	}

	logger.InfoContext(ctx, "Testing enhanced lock system")

	// Test lock functionality with dependencies
	if deps.Lock == nil {
		return fmt.Errorf("lock adapter not available: %w", ErrCritical)
	}
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available: %w", ErrCritical)
	}
	if deps.Process == nil {
		return fmt.Errorf("process adapter not available: %w", ErrCritical)
	}

	logger.InfoContext(ctx, "Lock adapter available")

	// Perform comprehensive lock tests
	testDir, err := deps.FileSystem.TempDir(ctx, "", "devback_test_locks_*")
	if err != nil {
		return fmt.Errorf("failed to create test directory: %w", ErrCritical)
	}
	defer func() {
		_ = deps.FileSystem.RemoveAll(ctx, testDir)
	}()

	lockPath := deps.FileSystem.Join(testDir, ".test.lock")

	logger.InfoContext(ctx, "Testing enhanced lock system...")

	// Test 1: Acquire lock
	logger.InfoContext(ctx, "Test 1: Acquiring lock...")
	lockInfo := LockInfo{
		PID:       deps.Process.GetPID(), // Use real process PID
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
		BackupDir: "/test/backup",
		Hostname:  "test-host",
	}

	err = deps.Lock.AcquireLock(ctx, lockPath, lockInfo)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock acquired successfully")

	// Test 2: Check if locked
	logger.InfoContext(ctx, "Test 2: Checking lock status...")
	isLocked, info, err := deps.Lock.IsLocked(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("failed to check lock status: %w", ErrCritical)
	}
	if !isLocked {
		return fmt.Errorf("lock should be active but is not detected: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock is active", "pid", info.PID, "hostname", info.Hostname)

	// Test 3: Try to acquire the same lock (should fail)
	logger.InfoContext(ctx, "Test 3: Trying to acquire same lock again...")
	conflictInfo := LockInfo{
		PID:       deps.Process.GetPID() + 1, // Different PID
		StartTime: time.Now(),
		RepoPath:  lockInfo.RepoPath,
		BackupDir: lockInfo.BackupDir,
		Hostname:  lockInfo.Hostname,
	}
	err = deps.Lock.AcquireLock(ctx, lockPath, conflictInfo)
	if err != nil {
		logger.InfoContext(ctx, "Correctly failed to acquire lock", "error", err)
	} else {
		return fmt.Errorf("should have failed to acquire lock: %w", ErrCritical)
	}

	// Test 4: Refresh lock
	logger.InfoContext(ctx, "Test 4: Refreshing lock...")
	err = deps.Lock.RefreshLock(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("failed to refresh lock: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock refreshed successfully")

	// Test 5: Release lock
	logger.InfoContext(ctx, "Test 5: Releasing lock...")
	err = deps.Lock.ReleaseLock(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock released successfully")

	// Test 6: Check lock after release
	logger.InfoContext(ctx, "Test 6: Checking lock after release...")
	isLocked, _, err = deps.Lock.IsLocked(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("failed to check lock status: %w", ErrCritical)
	}
	if isLocked {
		return fmt.Errorf("lock should not be active after release: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock is correctly released")

	// Test 7: Acquire lock again (should work now)
	logger.InfoContext(ctx, "Test 7: Acquiring lock after release...")
	newLockInfo := LockInfo{
		PID:       deps.Process.GetPID(),
		StartTime: time.Now(),
		RepoPath:  lockInfo.RepoPath,
		BackupDir: lockInfo.BackupDir,
		Hostname:  lockInfo.Hostname,
	}
	err = deps.Lock.AcquireLock(ctx, lockPath, newLockInfo)
	if err != nil {
		return fmt.Errorf("failed to acquire lock after release: %w", ErrCritical)
	}
	logger.InfoContext(ctx, "Lock acquired successfully after release")

	// Clean up
	_ = deps.Lock.ReleaseLock(ctx, lockPath)

	logger.InfoContext(ctx, "All lock tests completed successfully")
	logger.InfoContext(ctx, "Enhanced lock system operational")
	return nil
}

// PrintRepoKey handles the --print-repo-key flow.
func PrintRepoKey(ctx context.Context, cfg *Config, deps *Dependencies, logger *slog.Logger) (string, error) {
	if logger == nil {
		panic("logger is required")
	}

	logger.InfoContext(ctx, "Printing repository key")

	if deps.Git == nil {
		logger.ErrorContext(ctx, "Git adapter not available")
		return "", fmt.Errorf("git adapter not available: %w", ErrCritical)
	}

	repoRoot, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to resolve repository root", "error", err)
		return "", fmt.Errorf("failed to resolve repository root: %w", ErrCritical)
	}

	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		logger.ErrorContext(ctx, "Not a git repository", "error", err)
		return "", fmt.Errorf("not a git repository: %w", ErrCritical)
	}

	bc := newBackupContext(logger, cfg.Verbose)
	repoKey := deriveRepoKey(ctx, cfg, deps, repoRoot, bc)

	return repoKey, nil
}

// Backup handles the main backup functionality.
func Backup(ctx context.Context, cfg *Config, deps *Dependencies, logger *slog.Logger) (*BackupResult, error) {
	if logger == nil {
		panic("logger is required")
	}

	logger.InfoContext(
		ctx,
		"Starting backup operation",
		"backup_dir",
		cfg.BackupDir,
		"dry_run",
		cfg.DryRun,
	)
	bc := newBackupContext(logger, cfg.Verbose)

	if err := validateBackupDependencies(ctx, cfg, deps, logger); err != nil {
		return nil, err
	}

	printConfig(cfg, bc)

	repoRoot, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to resolve repository root", "error", err)
		return nil, fmt.Errorf("failed to resolve repository root: %w", ErrCritical)
	}

	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		logger.ErrorContext(ctx, "Not a git repository", "error", err)
		return nil, fmt.Errorf("not a git repository: %w", ErrCritical)
	}

	repoKey := deriveRepoKey(ctx, cfg, deps, repoRoot, bc)

	if cfg.DryRun {
		return handleDryRun(ctx, cfg, deps, repoRoot, repoKey, bc)
	}

	repoDir, err := ensureBackupDirs(ctx, deps, cfg, repoKey, logger)
	if err != nil {
		return nil, err
	}

	lockPath, releaseLock, err := acquireBackupLock(ctx, deps, repoDir, repoRoot, cfg, logger)
	if err != nil {
		return nil, err
	}
	defer releaseLock()

	stopRefresh := startLockRefresh(ctx, deps, lockPath, logger)
	defer stopRefresh()

	return handleBackupFlow(ctx, cfg, deps, repoRoot, repoDir, bc)
}

func validateBackupDependencies(
	ctx context.Context,
	cfg *Config,
	deps *Dependencies,
	logger *slog.Logger,
) error {
	if deps.FileSystem == nil {
		logger.ErrorContext(ctx, "FileSystem adapter not available")
		return fmt.Errorf("filesystem adapter not available: %w", ErrCritical)
	}
	if deps.Git == nil {
		logger.ErrorContext(ctx, "Git adapter not available")
		return fmt.Errorf("git adapter not available: %w", ErrCritical)
	}
	if cfg.DryRun {
		return nil
	}
	if deps.Lock == nil {
		logger.ErrorContext(ctx, "Lock adapter not available")
		return fmt.Errorf("lock adapter not available: %w", ErrCritical)
	}
	if deps.Process == nil {
		logger.ErrorContext(ctx, "Process adapter not available")
		return fmt.Errorf("process adapter not available: %w", ErrCritical)
	}
	return nil
}

func ensureBackupDirs(
	ctx context.Context,
	deps *Dependencies,
	cfg *Config,
	repoKey string,
	logger *slog.Logger,
) (string, error) {
	if err := deps.FileSystem.CreateDir(ctx, cfg.BackupDir, 0o755); err != nil {
		logger.ErrorContext(ctx, "ensure backup root", "error", err)
		return "", fmt.Errorf("ensure backup root: %w", ErrCritical)
	}
	repoDir := deps.FileSystem.Join(cfg.BackupDir, repoKey)
	if err := deps.FileSystem.CreateDir(ctx, repoDir, 0o755); err != nil {
		logger.ErrorContext(ctx, "ensure repo dir", "error", err)
		return "", fmt.Errorf("ensure repo dir: %w", ErrCritical)
	}
	return repoDir, nil
}

func acquireBackupLock(
	ctx context.Context,
	deps *Dependencies,
	repoDir,
	repoRoot string,
	cfg *Config,
	logger *slog.Logger,
) (string, func(), error) {
	lockPath := deps.FileSystem.Join(repoDir, ".backup.lock")
	lockInfo := LockInfo{
		PID:       deps.Process.GetPID(),
		StartTime: time.Now(),
		RepoPath:  repoRoot,
		BackupDir: cfg.BackupDir,
	}

	if err := deps.Lock.AcquireLock(ctx, lockPath, lockInfo); err != nil {
		logger.WarnContext(ctx, "Failed to acquire lock", "error", err)
		if strings.Contains(err.Error(), "lock is held") {
			return "", nil, ErrLockBusy
		}
		return "", nil, fmt.Errorf("failed to acquire lock: %w", ErrCritical)
	}

	release := func() {
		_ = deps.Lock.ReleaseLock(ctx, lockPath)
	}
	return lockPath, release, nil
}

func startLockRefresh(
	ctx context.Context,
	deps *Dependencies,
	lockPath string,
	logger *slog.Logger,
) func() {
	refreshCtx, stopRefresh := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(lockRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				if err := deps.Lock.RefreshLock(refreshCtx, lockPath); err != nil {
					logger.WarnContext(refreshCtx, "Failed to refresh lock", "error", err)
				}
			}
		}
	}()
	return stopRefresh
}
