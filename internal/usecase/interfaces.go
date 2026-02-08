package usecase

import (
	"context"
	"time"
)

// Dependencies represents all external dependencies needed by use cases
type Dependencies struct {
	FileSystem   FileSystemPort
	Git          GitPort
	Lock         LockPort
	Process      ProcessPort
	Config       ConfigPort
	Templates    TemplatesPort
	Notification NotificationPort
}

// Ports define the interfaces that use cases need (hexagonal architecture)

// FileSystemPort defines filesystem operations needed by use cases
type FileSystemPort interface {
	// Core file operations
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm int) error
	CreateDir(ctx context.Context, path string, perm int) error
	RemoveAll(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (FileInfo, error)
	Lstat(ctx context.Context, path string) (FileInfo, error)

	// Directory operations
	Walk(ctx context.Context, root string, walkFn WalkFunc) error
	ReadDir(ctx context.Context, path string) ([]DirEntry, error)
	Glob(ctx context.Context, pattern string) ([]string, error)
	CreateDirExclusive(ctx context.Context, path string, perm int) error

	// File operations
	Copy(ctx context.Context, src, dst string) error
	Move(ctx context.Context, src, dst string) error
	Readlink(ctx context.Context, path string) (string, error)
	Symlink(ctx context.Context, target, path string) error
	Chmod(ctx context.Context, path string, perm int) error
	Chtimes(ctx context.Context, path string, atime, mtime time.Time) error

	// Path operations
	GetWorkingDir(ctx context.Context) (string, error)
	Abs(ctx context.Context, path string) (string, error)
	Join(elements ...string) string
	Base(path string) string
	Dir(path string) string
	Ext(path string) string
	IsAbs(path string) bool
	Rel(basepath, targpath string) (string, error)
	Clean(path string) string
	VolumeName(path string) string
	PathSeparator() byte

	// Error classification
	IsNotExist(err error) bool
	IsExist(err error) bool
	IsPermission(err error) bool

	// Temp operations
	TempDir(ctx context.Context, dir, prefix string) (string, error)
}

// GitPort defines git operations needed by use cases
type GitPort interface {
	// RepoRoot returns repository root path
	RepoRoot(ctx context.Context) (string, error)

	// ConfigGet reads git config value
	ConfigGet(ctx context.Context, repoPath, key string) (string, error)

	// ConfigSet sets git config value
	ConfigSet(ctx context.Context, repoPath, key, value string) error

	// ConfigGetGlobal reads global git config value
	ConfigGetGlobal(ctx context.Context, key string) (string, error)

	// ConfigGetWorktree reads worktree config value
	ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error)

	// ConfigSetWorktree sets worktree config value
	ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error

	// ConfigSetGlobal sets global git config value
	ConfigSetGlobal(ctx context.Context, key, value string) error

	// GitDir returns the git directory for the repo
	GitDir(ctx context.Context, repoPath string) (string, error)

	// GitCommonDir returns the common git directory for the repo
	GitCommonDir(ctx context.Context, repoPath string) (string, error)

	// WorktreeList returns worktrees for repository.
	WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error)

	// ListIgnoredUntracked returns ignored/untracked files
	ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error)
}

// ConfigPort defines configuration operations needed by use cases
type ConfigPort interface {
	Load(ctx context.Context, path string) (ConfigFile, error)
	Save(ctx context.Context, path string, cfg ConfigFile) error
}

// TemplatesPort defines access to embedded templates
type TemplatesPort interface {
	List(ctx context.Context) ([]TemplateEntry, error)
	Read(ctx context.Context, name string) ([]byte, error)
	ListRepo(ctx context.Context) ([]TemplateEntry, error)
	ReadRepo(ctx context.Context, name string) ([]byte, error)
}

// LockPort defines locking operations needed by use cases
type LockPort interface {
	AcquireLock(ctx context.Context, path string, info LockInfo) error
	ReleaseLock(ctx context.Context, path string) error
	IsLocked(ctx context.Context, path string) (bool, LockInfo, error)
	RefreshLock(ctx context.Context, path string) error
}

// ProcessPort defines process operations needed by use cases
type ProcessPort interface {
	GetPID() int
}

// NotificationPort defines desktop notification operations needed by use cases
type NotificationPort interface {
	// Send sends a desktop notification. sound can be empty.
	Send(ctx context.Context, title, message, sound string) error
}
