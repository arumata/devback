package usecase

import "time"

// Config contains all application configuration
type Config struct {
	BackupDir         string
	Verbose           bool
	DryRun            bool
	PrintRepoKey      bool
	TestLocks         bool
	KeepCount         int
	KeepDays          int
	MaxTotalGBPerRepo int
	SizeMarginMB      int
	RepoKeyStyle      string
	AutoRemoteMerge   bool
	RemoteHashLen     int
	NoSize            bool
}

// FileInfo represents file information.
type FileInfo interface {
	Name() string
	Size() int64
	Mode() int
	ModTime() time.Time
	IsDir() bool
	IsSymlink() bool
	IsRegular() bool
	Sys() interface{}
}

// WalkFunc is called for each file/directory during Walk.
type WalkFunc func(path string, info FileInfo, err error) error

// DirEntry represents a directory entry.
type DirEntry interface {
	Name() string
	IsDir() bool
}

// Remote represents git remote.
type Remote struct {
	Name string
	URL  string
}

// WorktreeInfo describes a git worktree entry.
type WorktreeInfo struct {
	Path   string
	Branch string
}

// GitStatus represents repository status.
type GitStatus struct {
	Clean          bool
	ModifiedFiles  []string
	UntrackedFiles []string
	StagedFiles    []string
}

// GitCommit represents a git commit.
type GitCommit struct {
	Hash      string
	Author    string
	Date      time.Time
	Message   string
	ShortHash string
}

// LockInfo represents lock file information.
type LockInfo struct {
	PID               int       `json:"pid"`
	StartTime         time.Time `json:"start_time"`
	RepoPath          string    `json:"repo_path"`
	BackupDir         string    `json:"backup_dir"`
	Hostname          string    `json:"hostname"`
	ProcessStartTicks int64     `json:"process_start_ticks"`
	ProcessStartID    string    `json:"process_start_id"`
}

// ProcessInfo represents process information.
type ProcessInfo struct {
	PID        int
	Name       string
	StartTime  time.Time
	CPUPercent float64
	MemoryMB   int64
}

// BackupResult contains backup execution statistics
type BackupResult struct {
	TotalFiles     int
	CopiedFiles    int
	SkippedFiles   int
	SkippedDirs    int
	PermissionErrs []string
	OtherErrors    []string
	PartialSuccess bool
}
