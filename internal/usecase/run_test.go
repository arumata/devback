//nolint:gci,gofumpt
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Test constants
const (
	testBackupDir = "/test/backup"
	testRepoPath  = "/test/repo"
)

// Mock implementations for testing

type mockFileInfo struct {
	name    string
	size    int64
	mode    int
	modTime time.Time
	isDir   bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() int          { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) IsSymlink() bool    { return m.mode&int(os.ModeSymlink) != 0 }
func (m *mockFileInfo) IsRegular() bool    { return !m.isDir && m.mode&int(os.ModeType) == 0 }
func (m *mockFileInfo) Sys() interface{}   { return nil }

type mockFileSystem struct {
	CreateDirFunc     func(ctx context.Context, path string, perm int) error
	RemoveAllFunc     func(ctx context.Context, path string) error
	JoinFunc          func(elements ...string) string
	ReadFileFunc      func(ctx context.Context, path string) ([]byte, error)
	WriteFileFunc     func(ctx context.Context, path string, data []byte, perm int) error
	StatFunc          func(ctx context.Context, path string) (FileInfo, error)
	LstatFunc         func(ctx context.Context, path string) (FileInfo, error)
	WalkFunc          func(ctx context.Context, root string, walkFn WalkFunc) error
	ReadDirFunc       func(ctx context.Context, path string) ([]DirEntry, error)
	GlobFunc          func(ctx context.Context, pattern string) ([]string, error)
	CreateDirExclFunc func(ctx context.Context, path string, perm int) error
	CopyFunc          func(ctx context.Context, src, dst string) error
	MoveFunc          func(ctx context.Context, src, dst string) error
	ReadlinkFunc      func(ctx context.Context, path string) (string, error)
	SymlinkFunc       func(ctx context.Context, target, path string) error
	ChmodFunc         func(ctx context.Context, path string, perm int) error
	ChtimesFunc       func(ctx context.Context, path string, atime, mtime time.Time) error
	GetWorkingDirFunc func(ctx context.Context) (string, error)
	AbsFunc           func(ctx context.Context, path string) (string, error)
	BaseFunc          func(path string) string
	DirFunc           func(path string) string
	ExtFunc           func(path string) string
	TempDirFunc       func(ctx context.Context, dir, prefix string) (string, error)
	IsAbsFunc         func(path string) bool
	CleanFunc         func(path string) string
	IsNotExistFunc    func(err error) bool
}

func (m *mockFileSystem) CreateDir(ctx context.Context, path string, perm int) error {
	if m.CreateDirFunc != nil {
		return m.CreateDirFunc(ctx, path, perm)
	}
	return nil
}

func (m *mockFileSystem) RemoveAll(ctx context.Context, path string) error {
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(ctx, path)
	}
	return nil
}

func (m *mockFileSystem) Join(elements ...string) string {
	if m.JoinFunc != nil {
		return m.JoinFunc(elements...)
	}
	return filepath.Join(elements...)
}

func (m *mockFileSystem) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(ctx, path)
	}
	return nil, nil
}

func (m *mockFileSystem) WriteFile(ctx context.Context, path string, data []byte, perm int) error {
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(ctx, path, data, perm)
	}
	return nil
}

func (m *mockFileSystem) Stat(ctx context.Context, path string) (FileInfo, error) {
	if m.StatFunc != nil {
		return m.StatFunc(ctx, path)
	}
	return &mockFileInfo{isDir: true}, nil
}

func (m *mockFileSystem) Lstat(ctx context.Context, path string) (FileInfo, error) {
	if m.LstatFunc != nil {
		return m.LstatFunc(ctx, path)
	}
	return &mockFileInfo{}, nil
}

func (m *mockFileSystem) Walk(ctx context.Context, root string, walkFn WalkFunc) error {
	if m.WalkFunc != nil {
		return m.WalkFunc(ctx, root, walkFn)
	}
	return nil
}

func (m *mockFileSystem) ReadDir(ctx context.Context, path string) ([]DirEntry, error) {
	if m.ReadDirFunc != nil {
		return m.ReadDirFunc(ctx, path)
	}
	return nil, nil
}

func (m *mockFileSystem) Glob(ctx context.Context, pattern string) ([]string, error) {
	if m.GlobFunc != nil {
		return m.GlobFunc(ctx, pattern)
	}
	return nil, nil
}

func (m *mockFileSystem) CreateDirExclusive(ctx context.Context, path string, perm int) error {
	if m.CreateDirExclFunc != nil {
		return m.CreateDirExclFunc(ctx, path, perm)
	}
	return nil
}

func (m *mockFileSystem) Copy(ctx context.Context, src, dst string) error {
	if m.CopyFunc != nil {
		return m.CopyFunc(ctx, src, dst)
	}
	return nil
}

func (m *mockFileSystem) Move(ctx context.Context, src, dst string) error {
	if m.MoveFunc != nil {
		return m.MoveFunc(ctx, src, dst)
	}
	return nil
}

func (m *mockFileSystem) Readlink(ctx context.Context, path string) (string, error) {
	if m.ReadlinkFunc != nil {
		return m.ReadlinkFunc(ctx, path)
	}
	return "", nil
}

func (m *mockFileSystem) Symlink(ctx context.Context, target, path string) error {
	if m.SymlinkFunc != nil {
		return m.SymlinkFunc(ctx, target, path)
	}
	return nil
}

func (m *mockFileSystem) Chmod(ctx context.Context, path string, perm int) error {
	if m.ChmodFunc != nil {
		return m.ChmodFunc(ctx, path, perm)
	}
	return nil
}

func (m *mockFileSystem) Chtimes(ctx context.Context, path string, atime, mtime time.Time) error {
	if m.ChtimesFunc != nil {
		return m.ChtimesFunc(ctx, path, atime, mtime)
	}
	return nil
}

func (m *mockFileSystem) GetWorkingDir(ctx context.Context) (string, error) {
	if m.GetWorkingDirFunc != nil {
		return m.GetWorkingDirFunc(ctx)
	}
	return "/test", nil
}

func (m *mockFileSystem) Abs(ctx context.Context, path string) (string, error) {
	if m.AbsFunc != nil {
		return m.AbsFunc(ctx, path)
	}
	return path, nil
}

func (m *mockFileSystem) Base(path string) string {
	if m.BaseFunc != nil {
		return m.BaseFunc(path)
	}
	return "base"
}

func (m *mockFileSystem) Dir(path string) string {
	if m.DirFunc != nil {
		return m.DirFunc(path)
	}
	return "dir"
}

func (m *mockFileSystem) Ext(path string) string {
	if m.ExtFunc != nil {
		return m.ExtFunc(path)
	}
	return ".ext"
}

func (m *mockFileSystem) IsAbs(path string) bool {
	if m.IsAbsFunc != nil {
		return m.IsAbsFunc(path)
	}
	return strings.HasPrefix(path, "/")
}

func (m *mockFileSystem) Rel(basepath, targpath string) (string, error) {
	return strings.TrimPrefix(targpath, basepath+"/"), nil
}

func (m *mockFileSystem) Clean(path string) string {
	if m.CleanFunc != nil {
		return m.CleanFunc(path)
	}
	return path
}

func (m *mockFileSystem) VolumeName(_ string) string { return "" }
func (m *mockFileSystem) PathSeparator() byte        { return '/' }

func (m *mockFileSystem) IsNotExist(err error) bool {
	if m.IsNotExistFunc != nil {
		return m.IsNotExistFunc(err)
	}
	return errors.Is(err, os.ErrNotExist)
}

func (m *mockFileSystem) IsExist(err error) bool {
	return errors.Is(err, os.ErrExist)
}

func (m *mockFileSystem) IsPermission(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

func (m *mockFileSystem) TempDir(ctx context.Context, dir, prefix string) (string, error) {
	if m.TempDirFunc != nil {
		return m.TempDirFunc(ctx, dir, prefix)
	}
	return "/tmp/test", nil
}

type mockLock struct {
	AcquireLockFunc func(ctx context.Context, path string, info LockInfo) error
	ReleaseLockFunc func(ctx context.Context, path string) error
	IsLockedFunc    func(ctx context.Context, path string) (bool, LockInfo, error)
	RefreshLockFunc func(ctx context.Context, path string) error
}

func (m *mockLock) AcquireLock(ctx context.Context, path string, info LockInfo) error {
	if m.AcquireLockFunc != nil {
		return m.AcquireLockFunc(ctx, path, info)
	}
	return nil
}

func (m *mockLock) ReleaseLock(ctx context.Context, path string) error {
	if m.ReleaseLockFunc != nil {
		return m.ReleaseLockFunc(ctx, path)
	}
	return nil
}

func (m *mockLock) IsLocked(ctx context.Context, path string) (bool, LockInfo, error) {
	if m.IsLockedFunc != nil {
		return m.IsLockedFunc(ctx, path)
	}
	return false, LockInfo{}, nil
}

func (m *mockLock) RefreshLock(ctx context.Context, path string) error {
	if m.RefreshLockFunc != nil {
		return m.RefreshLockFunc(ctx, path)
	}
	return nil
}

type mockGit struct {
	RepoRootFunc             func(ctx context.Context) (string, error)
	ConfigGetFunc            func(ctx context.Context, repoPath, key string) (string, error)
	ConfigSetFunc            func(ctx context.Context, repoPath, key, value string) error
	ConfigGetGlobalFunc      func(ctx context.Context, key string) (string, error)
	ConfigGetWorktreeFunc    func(ctx context.Context, repoPath, key string) (string, error)
	ConfigSetWorktreeFunc    func(ctx context.Context, repoPath, key, value string) error
	ConfigSetGlobalFunc      func(ctx context.Context, key, value string) error
	ListIgnoredUntrackedFunc func(ctx context.Context, repoPath string) ([]string, error)
	GitDirFunc               func(ctx context.Context, repoPath string) (string, error)
	GitCommonDirFunc         func(ctx context.Context, repoPath string) (string, error)
	WorktreeListFunc         func(ctx context.Context, repoPath string) ([]WorktreeInfo, error)
}

func (m *mockGit) Init(ctx context.Context, path string) error                    { return nil }
func (m *mockGit) Add(ctx context.Context, repoPath string, files []string) error { return nil }
func (m *mockGit) Commit(ctx context.Context, repoPath, message string) error     { return nil }
func (m *mockGit) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	return "abc123", nil
}

func (m *mockGit) GetRemotes(ctx context.Context, repoPath string) ([]Remote, error) {
	return nil, nil
}

func (m *mockGit) GetBranches(ctx context.Context, repoPath string) ([]string, error) {
	return nil, nil
}

func (m *mockGit) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return "main", nil
}

func (m *mockGit) GetStatus(ctx context.Context, repoPath string) (GitStatus, error) {
	return GitStatus{}, nil
}

func (m *mockGit) GetLog(ctx context.Context, repoPath string, limit int) ([]GitCommit, error) {
	return nil, nil
}

func (m *mockGit) RepoRoot(ctx context.Context) (string, error) {
	if m.RepoRootFunc != nil {
		return m.RepoRootFunc(ctx)
	}
	return testRepoPath, nil
}

func (m *mockGit) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	if m.ConfigGetFunc != nil {
		return m.ConfigGetFunc(ctx, repoPath, key)
	}
	return "", fmt.Errorf("not found")
}

func (m *mockGit) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	if m.ConfigSetFunc != nil {
		return m.ConfigSetFunc(ctx, repoPath, key, value)
	}
	return nil
}

func (m *mockGit) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	if m.ConfigGetGlobalFunc != nil {
		return m.ConfigGetGlobalFunc(ctx, key)
	}
	return "", fmt.Errorf("not found")
}

func (m *mockGit) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	if m.ConfigGetWorktreeFunc != nil {
		return m.ConfigGetWorktreeFunc(ctx, repoPath, key)
	}
	return "", fmt.Errorf("not found")
}

func (m *mockGit) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	if m.ConfigSetWorktreeFunc != nil {
		return m.ConfigSetWorktreeFunc(ctx, repoPath, key, value)
	}
	return nil
}

func (m *mockGit) ConfigSetGlobal(ctx context.Context, key, value string) error {
	if m.ConfigSetGlobalFunc != nil {
		return m.ConfigSetGlobalFunc(ctx, key, value)
	}
	return nil
}

func (m *mockGit) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	if m.ListIgnoredUntrackedFunc != nil {
		return m.ListIgnoredUntrackedFunc(ctx, repoPath)
	}
	return nil, nil
}

func (m *mockGit) GitDir(ctx context.Context, repoPath string) (string, error) {
	if m.GitDirFunc != nil {
		return m.GitDirFunc(ctx, repoPath)
	}
	return ".git", nil
}

func (m *mockGit) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	if m.GitCommonDirFunc != nil {
		return m.GitCommonDirFunc(ctx, repoPath)
	}
	return ".git", nil
}

func (m *mockGit) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	if m.WorktreeListFunc != nil {
		return m.WorktreeListFunc(ctx, repoPath)
	}
	return nil, nil
}
func (m *mockGit) Fetch(ctx context.Context, repoPath, remote string) error          { return nil }
func (m *mockGit) Push(ctx context.Context, repoPath, remote, branch string) error   { return nil }
func (m *mockGit) CheckoutBranch(ctx context.Context, repoPath, branch string) error { return nil }
func (m *mockGit) IsClean(ctx context.Context, repoPath string) (bool, error)        { return true, nil }

type mockProcess struct{}

func (m *mockProcess) GetPID() int { return 12345 }

func TestHandleBackup_RefreshesLock(t *testing.T) {
	originalInterval := lockRefreshInterval
	lockRefreshInterval = 5 * time.Millisecond
	defer func() {
		lockRefreshInterval = originalInterval
	}()

	refreshCh := make(chan struct{})
	blockCh := make(chan struct{})
	var refreshOnce sync.Once
	mockLockImpl := &mockLock{
		RefreshLockFunc: func(ctx context.Context, path string) error {
			refreshOnce.Do(func() {
				close(refreshCh)
			})
			return nil
		},
	}

	createDirCalls := 0
	mockFS := &mockFileSystem{
		CreateDirFunc: func(ctx context.Context, path string, perm int) error {
			createDirCalls++
			if createDirCalls == 3 {
				select {
				case <-blockCh:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return &mockFileInfo{isDir: true}, nil
		},
		ReadFileFunc: func(ctx context.Context, path string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			return nil
		},
		ReadDirFunc: func(ctx context.Context, path string) ([]DirEntry, error) {
			return nil, nil
		},
	}

	mockGitImpl := &mockGit{
		RepoRootFunc: func(ctx context.Context) (string, error) {
			return testRepoPath, nil
		},
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			return "", os.ErrNotExist
		},
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return nil, nil
		},
	}

	deps := &Dependencies{
		FileSystem: mockFS,
		Git:        mockGitImpl,
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}
	cfg := &Config{BackupDir: "/backup", NoSize: true}

	done := make(chan error, 1)
	go func() {
		_, err := Backup(context.Background(), cfg, deps, slog.Default())
		done <- err
	}()

	select {
	case <-refreshCh:
		close(blockCh)
	case <-time.After(200 * time.Millisecond):
		close(blockCh)
		t.Fatal("expected lock refresh to be triggered")
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, ErrInterrupted) && !errors.Is(err, ErrCritical) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("backup did not finish in time")
	}
}

func TestHandleBackup_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockFS := &mockFileSystem{
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return &mockFileInfo{isDir: true}, nil
		},
	}
	mockGitImpl := &mockGit{
		RepoRootFunc: func(ctx context.Context) (string, error) {
			return testRepoPath, nil
		},
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			return "", os.ErrNotExist
		},
	}

	_, err := Backup(ctx, &Config{BackupDir: "/backup"}, &Dependencies{
		FileSystem: mockFS,
		Git:        mockGitImpl,
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}, slog.Default())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected interrupted error, got %v", err)
	}
}

// createTestLockWithState creates a mock lock with state that tracks lock acquire/release
func createTestLockWithState() (*mockLock, **LockInfo) {
	var currentLockInfo *LockInfo
	mockLockImpl := &mockLock{
		AcquireLockFunc: func(ctx context.Context, path string, info LockInfo) error {
			if currentLockInfo != nil && currentLockInfo.PID != info.PID {
				return fmt.Errorf("lock is already held by PID %d", currentLockInfo.PID)
			}
			currentLockInfo = &info
			return nil
		},
		IsLockedFunc: func(ctx context.Context, path string) (bool, LockInfo, error) {
			if currentLockInfo != nil {
				return true, *currentLockInfo, nil
			}
			return false, LockInfo{}, nil
		},
		ReleaseLockFunc: func(ctx context.Context, path string) error {
			currentLockInfo = nil
			return nil
		},
	}
	return mockLockImpl, &currentLockInfo
}

func TestBackup_WithBackupDir(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{BackupDir: testBackupDir}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	_, err := Backup(ctx, cfg, deps, slog.Default())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestTestLocks_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}

	mockFS := &mockFileSystem{}
	mockLockImpl, currentLockInfo := createTestLockWithState()

	deps := &Dependencies{
		FileSystem: mockFS,
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Clean up to avoid affecting other tests
	*currentLockInfo = nil
}

func TestTestLocks_NoLockAdapter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Lock:       nil, // No lock adapter
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestPrintRepoKey(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestPrintRepoKey_NoGitAdapter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        nil,
	}

	if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestPrintRepoKey_InvalidRepo(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	mockFS := &mockFileSystem{
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			if strings.HasSuffix(path, ".git") {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{isDir: true}, nil
		},
	}
	deps := &Dependencies{
		FileSystem: mockFS,
		Git:        &mockGit{},
	}

	if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestPrintRepoKey_RepoRootError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	mockGit := &mockGit{
		RepoRootFunc: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("no repo")
		},
	}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        mockGit,
	}

	if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestHandleBackup_LockBusy(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		BackupDir: "/backup",
	}
	mockFS := &mockFileSystem{
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return &mockFileInfo{isDir: true}, nil
		},
	}
	mockLockImpl := &mockLock{
		AcquireLockFunc: func(ctx context.Context, path string, info LockInfo) error {
			return fmt.Errorf("lock is held by another active process")
		},
	}
	deps := &Dependencies{
		FileSystem: mockFS,
		Git:        &mockGit{},
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	_, err := Backup(ctx, cfg, deps, slog.Default())
	if !errors.Is(err, ErrLockBusy) {
		t.Errorf("Expected lock busy error, got %v", err)
	}
}

func TestHandleBackup_MissingAdapters(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		BackupDir: "/backup",
	}

	_, err := Backup(ctx, cfg, &Dependencies{Git: &mockGit{}, Lock: &mockLock{}, Process: &mockProcess{}}, slog.Default())
	if !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}

	_, err = Backup(
		ctx,
		cfg,
		&Dependencies{
			FileSystem: &mockFileSystem{},
			Lock:       &mockLock{},
			Process:    &mockProcess{},
		},
		slog.Default(),
	)
	if !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}

	_, err = Backup(
		ctx,
		cfg,
		&Dependencies{
			FileSystem: &mockFileSystem{},
			Git:        &mockGit{},
			Process:    &mockProcess{},
		},
		slog.Default(),
	)
	if !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}

	_, err = Backup(
		ctx,
		cfg,
		&Dependencies{
			FileSystem: &mockFileSystem{},
			Git:        &mockGit{},
			Lock:       &mockLock{},
		},
		slog.Default(),
	)
	if !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleBackup_InvalidRepo(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		BackupDir: "/backup",
	}
	mockFS := &mockFileSystem{
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			if strings.HasSuffix(path, ".git") {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{isDir: true}, nil
		},
	}
	deps := &Dependencies{
		FileSystem: mockFS,
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	_, err := Backup(ctx, cfg, deps, slog.Default())
	if !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_NoFileSystem(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: nil,
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_NoProcess(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Lock:       &mockLock{},
		Process:    nil,
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_CreateDirFailure(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}

	mockFS := &mockFileSystem{
		CreateDirFunc: func(ctx context.Context, path string, perm int) error {
			return fmt.Errorf("permission denied")
		},
	}

	deps := &Dependencies{
		FileSystem: mockFS,
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_AcquireLockFailure(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}

	mockLockImpl := &mockLock{
		AcquireLockFunc: func(ctx context.Context, path string, info LockInfo) error {
			return fmt.Errorf("lock acquire failed")
		},
	}

	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_IsLockedFailure(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}

	mockLockImpl := &mockLock{
		IsLockedFunc: func(ctx context.Context, path string) (bool, LockInfo, error) {
			return false, LockInfo{}, fmt.Errorf("check lock failed")
		},
	}

	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleTestLocks_LockNotDetected(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}

	mockLockImpl := &mockLock{
		IsLockedFunc: func(ctx context.Context, path string) (bool, LockInfo, error) {
			return false, LockInfo{}, nil // Lock not detected
		},
	}

	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	if err := TestLocks(ctx, cfg, deps, slog.Default()); !errors.Is(err, ErrCritical) {
		t.Errorf("Expected critical error, got %v", err)
	}
}

func TestHandleBackup(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		BackupDir: "/test/backup",
		DryRun:    true,
	}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	if _, err := Backup(ctx, cfg, deps, slog.Default()); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestPrintRepoKey_Basic(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// Benchmark tests for performance monitoring

func BenchmarkBackup_Basic(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{BackupDir: testBackupDir}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Backup(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkTestLocks(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{}

	mockFS := &mockFileSystem{}
	mockLockImpl, currentLockInfo := createTestLockWithState()

	deps := &Dependencies{
		FileSystem: mockFS,
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := TestLocks(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
		// Reset lock state for next iteration
		*currentLockInfo = nil
	}
}

func BenchmarkPrintRepoKey(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkHandleTestLocks_FullCycle(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{}

	mockFS := &mockFileSystem{}
	mockLockImpl, currentLockInfo := createTestLockWithState()
	mockLockImpl.RefreshLockFunc = func(ctx context.Context, path string) error {
		return nil
	}

	deps := &Dependencies{
		FileSystem: mockFS,
		Lock:       mockLockImpl,
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := TestLocks(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
		// Reset lock state for next iteration
		*currentLockInfo = nil
	}
}

func BenchmarkHandleBackup(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{
		BackupDir: "/test/backup",
		DryRun:    true,
	}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Backup(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkPrintRepoKey_Basic(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{}
	deps := &Dependencies{
		FileSystem: &mockFileSystem{},
		Git:        &mockGit{},
		Lock:       &mockLock{},
		Process:    &mockProcess{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := PrintRepoKey(ctx, cfg, deps, slog.Default()); err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}
