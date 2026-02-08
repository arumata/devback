// Package noop provides placeholder implementations for all adapter interfaces
package noop

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

// Adapter implements all adapter interfaces with no-op implementations
// This is used as placeholder until real adapters are implemented
type Adapter struct {
	logger *slog.Logger
}

var errNotImplemented = errors.New("operation not implemented in no-op adapter")

// ReadFile returns error for filesystem operations
func (a Adapter) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return nil, errNotImplemented
}

// WriteFile returns error for filesystem operations
func (a Adapter) WriteFile(ctx context.Context, path string, data []byte, perm int) error {
	return errNotImplemented
}

// CreateDir returns error for filesystem operations
func (a Adapter) CreateDir(ctx context.Context, path string, perm int) error {
	return errNotImplemented
}

// RemoveAll returns error for filesystem operations
func (a Adapter) RemoveAll(ctx context.Context, path string) error {
	return errNotImplemented
}

// Stat returns error for filesystem operations
func (a Adapter) Stat(ctx context.Context, path string) (usecase.FileInfo, error) {
	return nil, errNotImplemented
}

// Lstat returns error for filesystem operations
func (a Adapter) Lstat(ctx context.Context, path string) (usecase.FileInfo, error) {
	return nil, errNotImplemented
}

// Walk returns error for filesystem operations
func (a Adapter) Walk(ctx context.Context, root string, walkFn usecase.WalkFunc) error {
	return errNotImplemented
}

// ReadDir returns error for filesystem operations
func (a Adapter) ReadDir(ctx context.Context, path string) ([]usecase.DirEntry, error) {
	return nil, errNotImplemented
}

// Glob returns error for filesystem operations
func (a Adapter) Glob(ctx context.Context, pattern string) ([]string, error) {
	return nil, errNotImplemented
}

// CreateDirExclusive returns error for filesystem operations
func (a Adapter) CreateDirExclusive(ctx context.Context, path string, perm int) error {
	return errNotImplemented
}

// Copy returns error for filesystem operations
func (a Adapter) Copy(ctx context.Context, src, dst string) error {
	return errNotImplemented
}

// Move returns error for filesystem operations
func (a Adapter) Move(ctx context.Context, src, dst string) error {
	return errNotImplemented
}

// Readlink returns error for filesystem operations
func (a Adapter) Readlink(ctx context.Context, path string) (string, error) {
	return "", errNotImplemented
}

// Symlink returns error for filesystem operations
func (a Adapter) Symlink(ctx context.Context, target, path string) error {
	return errNotImplemented
}

// Chmod returns error for filesystem operations
func (a Adapter) Chmod(ctx context.Context, path string, perm int) error {
	return errNotImplemented
}

// Chtimes returns error for filesystem operations
func (a Adapter) Chtimes(ctx context.Context, path string, atime, mtime time.Time) error {
	return errNotImplemented
}

// GetWorkingDir returns error for filesystem operations
func (a Adapter) GetWorkingDir(ctx context.Context) (string, error) {
	return "", errNotImplemented
}

// Abs returns error for filesystem operations
func (a Adapter) Abs(ctx context.Context, path string) (string, error) {
	return "", errNotImplemented
}

// Join returns empty string for filesystem operations
func (a Adapter) Join(elements ...string) string {
	return ""
}

// Base returns empty string for filesystem operations
func (a Adapter) Base(path string) string {
	return ""
}

// Dir returns empty string for filesystem operations
func (a Adapter) Dir(path string) string {
	return ""
}

// Ext returns empty string for filesystem operations
func (a Adapter) Ext(path string) string {
	return ""
}

// IsAbs returns false for filesystem operations
func (a Adapter) IsAbs(path string) bool {
	return false
}

// Rel returns error for filesystem operations
func (a Adapter) Rel(basepath, targpath string) (string, error) {
	return "", errNotImplemented
}

// Clean returns empty string for filesystem operations
func (a Adapter) Clean(path string) string {
	return ""
}

// VolumeName returns empty string for filesystem operations
func (a Adapter) VolumeName(path string) string {
	return ""
}

// PathSeparator returns '/' for filesystem operations
func (a Adapter) PathSeparator() byte {
	return '/'
}

// IsNotExist returns false for filesystem operations
func (a Adapter) IsNotExist(err error) bool {
	return false
}

// IsExist returns false for filesystem operations
func (a Adapter) IsExist(err error) bool {
	return false
}

// IsPermission returns false for filesystem operations
func (a Adapter) IsPermission(err error) bool {
	return false
}

// TempDir returns error for filesystem operations
func (a Adapter) TempDir(ctx context.Context, dir, prefix string) (string, error) {
	return "", errNotImplemented
}

// Init returns error for git operations
func (a Adapter) Init(ctx context.Context, path string) error {
	return errNotImplemented
}

// Add returns error for git operations
func (a Adapter) Add(ctx context.Context, repoPath string, files []string) error {
	return errNotImplemented
}

// Commit returns error for git operations
func (a Adapter) Commit(ctx context.Context, repoPath, message string) error {
	return errNotImplemented
}

// GetCommitHash returns error for git operations
func (a Adapter) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	return "", errNotImplemented
}

// GetRemotes returns error for git operations
func (a Adapter) GetRemotes(ctx context.Context, repoPath string) ([]usecase.Remote, error) {
	return nil, errNotImplemented
}

// Fetch returns error for git operations
func (a Adapter) Fetch(ctx context.Context, repoPath, remote string) error {
	return errNotImplemented
}

// Push returns error for git operations
func (a Adapter) Push(ctx context.Context, repoPath, remote, branch string) error {
	return errNotImplemented
}

// GetBranches returns error for git operations
func (a Adapter) GetBranches(ctx context.Context, repoPath string) ([]string, error) {
	return nil, errNotImplemented
}

// GetCurrentBranch returns error for git operations
func (a Adapter) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return "", errNotImplemented
}

// CheckoutBranch returns error for git operations
func (a Adapter) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	return errNotImplemented
}

// IsClean returns error for git operations
func (a Adapter) IsClean(ctx context.Context, repoPath string) (bool, error) {
	return false, errNotImplemented
}

// GetStatus returns error for git operations
func (a Adapter) GetStatus(ctx context.Context, repoPath string) (usecase.GitStatus, error) {
	return usecase.GitStatus{}, errNotImplemented
}

// GetLog returns error for git operations
func (a Adapter) GetLog(ctx context.Context, repoPath string, limit int) ([]usecase.GitCommit, error) {
	return nil, errNotImplemented
}

// RepoRoot returns error for git operations
func (a Adapter) RepoRoot(ctx context.Context) (string, error) {
	return "", errNotImplemented
}

// ConfigGet returns error for git operations
func (a Adapter) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	return "", errNotImplemented
}

// ConfigSet returns error for git operations
func (a Adapter) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	return errNotImplemented
}

// ConfigGetGlobal returns error for git operations
func (a Adapter) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	return "", errNotImplemented
}

// ConfigGetWorktree returns error for git operations
func (a Adapter) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	return "", errNotImplemented
}

// ConfigSetWorktree returns error for git operations
func (a Adapter) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	return errNotImplemented
}

// ConfigSetGlobal returns error for git operations
func (a Adapter) ConfigSetGlobal(ctx context.Context, key, value string) error {
	return errNotImplemented
}

// GitDir returns error for git operations
func (a Adapter) GitDir(ctx context.Context, repoPath string) (string, error) {
	return "", errNotImplemented
}

// GitCommonDir returns error for git operations
func (a Adapter) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	return "", errNotImplemented
}

// WorktreeList returns error for git operations
func (a Adapter) WorktreeList(ctx context.Context, repoPath string) ([]usecase.WorktreeInfo, error) {
	return nil, errNotImplemented
}

// ListIgnoredUntracked returns error for git operations
func (a Adapter) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	return nil, errNotImplemented
}

// Load returns error for config operations
func (a Adapter) Load(ctx context.Context, path string) (usecase.ConfigFile, error) {
	return usecase.ConfigFile{}, errNotImplemented
}

// Save returns error for config operations
func (a Adapter) Save(ctx context.Context, path string, cfg usecase.ConfigFile) error {
	return errNotImplemented
}

// List returns error for templates operations
func (a Adapter) List(ctx context.Context) ([]usecase.TemplateEntry, error) {
	return nil, errNotImplemented
}

// Read returns error for templates operations
func (a Adapter) Read(ctx context.Context, name string) ([]byte, error) {
	return nil, errNotImplemented
}

// ListRepo returns error for templates operations
func (a Adapter) ListRepo(ctx context.Context) ([]usecase.TemplateEntry, error) {
	return nil, errNotImplemented
}

// ReadRepo returns error for templates operations
func (a Adapter) ReadRepo(ctx context.Context, name string) ([]byte, error) {
	return nil, errNotImplemented
}

// AcquireLock returns error for lock operations
func (a Adapter) AcquireLock(ctx context.Context, path string, info usecase.LockInfo) error {
	return errNotImplemented
}

// ReleaseLock returns error for lock operations
func (a Adapter) ReleaseLock(ctx context.Context, path string) error {
	return errNotImplemented
}

// IsLocked returns error for lock operations
func (a Adapter) IsLocked(ctx context.Context, path string) (bool, usecase.LockInfo, error) {
	return false, usecase.LockInfo{}, errNotImplemented
}

// RefreshLock returns error for lock operations
func (a Adapter) RefreshLock(ctx context.Context, path string) error {
	return errNotImplemented
}

// GetPID returns zero for process operations
func (a Adapter) GetPID() int {
	return 0
}

// New creates a new no-op adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("noop adapter requires logger")
	}
	return &Adapter{logger: logger}
}
