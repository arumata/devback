//nolint:gci,gofumpt
package it

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arumata/devback/internal/app"
	"github.com/arumata/devback/internal/usecase"
)

// TestUsecaseWithRealAdapters tests the integration between usecase layer and real adapters
func TestUsecaseWithRealAdapters(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for test operations
	tempDir, err := os.MkdirTemp("", "devback-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create real dependencies
	deps := app.NewDefaultDependencies(slog.Default())

	t.Run("HandleTestLocks_RealLockAdapter", func(t *testing.T) {
		// Test the lock functionality with real adapters
		cfg := &usecase.Config{}

		// Run the usecase with real dependencies
		if err := usecase.TestLocks(ctx, cfg, deps, slog.Default()); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandleBackup_RealFileSystemAdapter", func(t *testing.T) {
		repoPath := setupIntegrationRepo(t, tempDir)
		restoreDir := chdirForTest(t, repoPath)
		defer restoreDir()

		// Test backup functionality with real filesystem adapter
		backupDir := filepath.Join(tempDir, "backup")
		cfg := &usecase.Config{
			BackupDir: backupDir,
			DryRun:    true, // Use dry run to avoid actual file operations
		}

		if _, err := usecase.Backup(ctx, cfg, deps, slog.Default()); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the backup directory was processed
		if cfg.BackupDir != backupDir {
			t.Errorf("Expected backup dir to be set to %s, got %s", backupDir, cfg.BackupDir)
		}
	})

	t.Run("HandlePrintRepoKey_RealGitAdapter", func(t *testing.T) {
		repoPath := setupIntegrationRepo(t, tempDir)
		restoreDir := chdirForTest(t, repoPath)
		defer restoreDir()

		// Test repo key functionality with real git adapter
		cfg := &usecase.Config{
			PrintRepoKey: true,
		}

		if _, err := usecase.PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitSetupStatus_RealAdapters(t *testing.T) {
	ctx := context.Background()
	deps := app.NewDefaultDependencies(slog.Default())

	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0o750); err != nil {
		t.Fatalf("Failed to create home dir: %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(homeDir, ".gitconfig"))

	repoPath := setupIntegrationRepo(t, tempDir)
	restoreDir := chdirForTest(t, repoPath)
	defer restoreDir()

	initOpts := usecase.InitOptions{
		HomeDir:    homeDir,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	if err := usecase.Init(ctx, initOpts, deps, slog.Default()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Expected config to exist: %v", err)
	}
	templatesDir := filepath.Join(homeDir, ".local", "share", "devback", "templates", "hooks")
	if _, err := os.Stat(templatesDir); err != nil {
		t.Fatalf("Expected templates to exist: %v", err)
	}

	if err := usecase.Setup(ctx, usecase.SetupOptions{HomeDir: homeDir}, deps, slog.Default()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git", "hooks", "post-commit")); err != nil {
		t.Fatalf("Expected hook to exist: %v", err)
	}
	if got, err := deps.Git.ConfigGet(ctx, repoPath, "backup.enabled"); err != nil || got != "true" {
		t.Fatalf("Expected backup.enabled=true, got %q (err=%v)", got, err)
	}

	report, err := usecase.Status(ctx, usecase.StatusOptions{HomeDir: homeDir}, deps, slog.Default())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if report.Repo == nil {
		t.Fatalf("Expected repo status to be populated")
	}
	if !report.Global.ConfigFile.Exists {
		t.Fatalf("Expected config to be reported as existing")
	}
	if report.Repo.Hooks.Installed == 0 {
		t.Fatalf("Expected hooks to be installed")
	}
}

func setupIntegrationRepo(t *testing.T, tempDir string) string {
	repoPath := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o600); err != nil {
		t.Fatal(err)
	}

	return repoPath
}

func chdirForTest(t *testing.T, dir string) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() {
		_ = os.Chdir(wd)
	}
}

// TestFileSystemAndLockInteraction tests interactions between filesystem and lock adapters
func TestFileSystemAndLockInteraction(t *testing.T) {
	ctx := context.Background()
	deps := app.NewDefaultDependencies(slog.Default())

	// Create a temporary directory
	tempDir, err := deps.FileSystem.TempDir(ctx, "", "devback-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = deps.FileSystem.RemoveAll(ctx, tempDir)
	}()

	// Create a lock file path
	lockPath := deps.FileSystem.Join(tempDir, "test.lock")

	// Test lock acquisition (use real PID for validation)
	lockInfo := usecase.LockInfo{
		PID:       os.Getpid(), // Use actual process PID
		StartTime: time.Now(),
		RepoPath:  tempDir,
		BackupDir: tempDir,
		Hostname:  "test-host",
	}

	err = deps.Lock.AcquireLock(ctx, lockPath, lockInfo)
	if err != nil {
		t.Errorf("Failed to acquire lock: %v", err)
	}

	// Test lock status check
	isLocked, info, err := deps.Lock.IsLocked(ctx, lockPath)
	if err != nil {
		t.Errorf("Failed to check lock status: %v", err)
	}
	if !isLocked {
		t.Error("Lock should be active")
	}
	if info.PID != lockInfo.PID {
		t.Errorf("Expected PID %d, got %d", lockInfo.PID, info.PID)
	}

	// Test lock release
	err = deps.Lock.ReleaseLock(ctx, lockPath)
	if err != nil {
		t.Errorf("Failed to release lock: %v", err)
	}

	// Verify lock is released
	isLocked, _, err = deps.Lock.IsLocked(ctx, lockPath)
	if err != nil {
		t.Errorf("Failed to check lock status after release: %v", err)
	}
	if isLocked {
		t.Error("Lock should not be active after release")
	}
}

// TestFileSystemOperations tests filesystem operations
func TestFileSystemOperations(t *testing.T) {
	ctx := context.Background()
	deps := app.NewDefaultDependencies(slog.Default())

	// Create a temporary directory
	tempDir, err := deps.FileSystem.TempDir(ctx, "", "devback-fs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = deps.FileSystem.RemoveAll(ctx, tempDir)
	}()

	// Test directory creation
	testDir := deps.FileSystem.Join(tempDir, "subdir")
	err = deps.FileSystem.CreateDir(ctx, testDir, 0o755)
	if err != nil {
		t.Errorf("Failed to create directory: %v", err)
	}

	// Test file write/read
	testFile := deps.FileSystem.Join(testDir, "test.txt")
	testContent := []byte("Hello, integration test!")

	err = deps.FileSystem.WriteFile(ctx, testFile, testContent, 0o644)
	if err != nil {
		t.Errorf("Failed to write file: %v", err)
	}

	readContent, err := deps.FileSystem.ReadFile(ctx, testFile)
	if err != nil {
		t.Errorf("Failed to read file: %v", err)
	}

	if string(readContent) != string(testContent) {
		t.Errorf("Expected content %q, got %q", string(testContent), string(readContent))
	}

	// Test file info
	info, err := deps.FileSystem.Stat(ctx, testFile)
	if err != nil {
		t.Errorf("Failed to stat file: %v", err)
	}

	if info.Size() != int64(len(testContent)) {
		t.Errorf("Expected file size %d, got %d", len(testContent), info.Size())
	}
}

// TestErrorScenarios tests error handling in integration scenarios
func TestErrorScenarios(t *testing.T) {
	ctx := context.Background()
	deps := app.NewDefaultDependencies(slog.Default())

	t.Run("LockConflict", func(t *testing.T) {
		// Create a temporary directory
		tempDir, err := deps.FileSystem.TempDir(ctx, "", "devback-lock-conflict-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() {
			_ = deps.FileSystem.RemoveAll(ctx, tempDir)
		}()

		lockPath := deps.FileSystem.Join(tempDir, "conflict.lock")

		// First lock acquisition
		lockInfo1 := usecase.LockInfo{
			PID:       os.Getpid(), // Use actual process PID
			StartTime: time.Now(),
			RepoPath:  tempDir,
			BackupDir: tempDir,
			Hostname:  "host1",
		}

		err = deps.Lock.AcquireLock(ctx, lockPath, lockInfo1)
		if err != nil {
			t.Fatalf("Failed to acquire first lock: %v", err)
		}

		// Attempt second lock acquisition (should fail)
		lockInfo2 := usecase.LockInfo{
			PID:       os.Getpid() + 1, // Different PID but still realistic
			StartTime: time.Now(),
			RepoPath:  tempDir,
			BackupDir: tempDir,
			Hostname:  "host2",
		}

		err = deps.Lock.AcquireLock(ctx, lockPath, lockInfo2)
		if err == nil {
			t.Error("Expected lock acquisition to fail due to conflict")
		}

		// Clean up
		_ = deps.Lock.ReleaseLock(ctx, lockPath)
	})

	t.Run("FilePermissionErrors", func(t *testing.T) {
		// Test handling of permission errors
		invalidPath := "/root/cannot-access-this-path"

		_, err := deps.FileSystem.ReadFile(ctx, invalidPath)
		if err == nil {
			t.Error("Expected read to fail for invalid path")
		}

		err = deps.FileSystem.WriteFile(ctx, invalidPath, []byte("test"), 0o644)
		if err == nil {
			t.Error("Expected write to fail for invalid path")
		}
	})
}

// TestConcurrentOperations tests concurrent access scenarios
func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	deps := app.NewDefaultDependencies(slog.Default())

	t.Run("ConcurrentLockOperations", func(t *testing.T) {
		// Create a temporary directory
		tempDir, err := deps.FileSystem.TempDir(ctx, "", "devback-concurrent-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() {
			_ = deps.FileSystem.RemoveAll(ctx, tempDir)
		}()

		lockPath := deps.FileSystem.Join(tempDir, "concurrent.lock")

		// Test concurrent lock attempts
		done := make(chan bool, 2)
		errors := make(chan error, 2)

		// First goroutine acquires lock
		go func() {
			defer func() { done <- true }()
			lockInfo := usecase.LockInfo{
				PID:       os.Getpid(),
				StartTime: time.Now(),
				RepoPath:  tempDir,
				BackupDir: tempDir,
				Hostname:  "concurrent1",
			}

			err := deps.Lock.AcquireLock(ctx, lockPath, lockInfo)
			if err != nil {
				errors <- err
				return
			}

			// Hold lock briefly
			time.Sleep(100 * time.Millisecond)

			err = deps.Lock.ReleaseLock(ctx, lockPath)
			if err != nil {
				errors <- err
			}
		}()

		// Second goroutine tries to acquire same lock
		go func() {
			defer func() { done <- true }()
			// Small delay to ensure first goroutine acquires lock first
			time.Sleep(50 * time.Millisecond)

			lockInfo := usecase.LockInfo{
				PID:       os.Getpid(),
				StartTime: time.Now(),
				RepoPath:  tempDir,
				BackupDir: tempDir,
				Hostname:  "concurrent2",
			}

			err := deps.Lock.AcquireLock(ctx, lockPath, lockInfo)
			if err == nil {
				// If we got the lock, release it
				_ = deps.Lock.ReleaseLock(ctx, lockPath)
			}
			// Note: We don't send error to errors channel here because
			// it's expected that one of the concurrent attempts might fail
		}()

		// Wait for both goroutines
		<-done
		<-done

		// Check if there were any unexpected errors
		close(errors)
		for err := range errors {
			t.Errorf("Unexpected error in concurrent operations: %v", err)
		}
	})
}
