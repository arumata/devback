//nolint:gci,gofumpt
package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

// Adapter implements GitAdapter using git command line tool
type Adapter struct {
	logger *slog.Logger
}

// New creates a new git adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("git adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// Init initializes git repository
func (a *Adapter) Init(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = path
	return cmd.Run()
}

// Add adds files to git index
func (a *Adapter) Add(ctx context.Context, repoPath string, files []string) error {
	args := append([]string{"add"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	return cmd.Run()
}

// Commit creates git commit
func (a *Adapter) Commit(ctx context.Context, repoPath, message string) error {
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	cmd.Dir = repoPath
	return cmd.Run()
}

// GetCommitHash returns current HEAD commit hash
func (a *Adapter) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetRemotes returns list of git remotes
func (a *Adapter) GetRemotes(ctx context.Context, repoPath string) ([]usecase.Remote, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "-v")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	remotesMap := make(map[string]string)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(line, "(fetch)") {
			remotesMap[parts[0]] = parts[1]
		}
	}

	remotes := make([]usecase.Remote, 0, len(remotesMap))
	for name, url := range remotesMap {
		remotes = append(remotes, usecase.Remote{
			Name: name,
			URL:  url,
		})
	}

	return remotes, nil
}

// Fetch fetches from remote
func (a *Adapter) Fetch(ctx context.Context, repoPath, remote string) error {
	cmd := exec.CommandContext(ctx, "git", "fetch", remote)
	cmd.Dir = repoPath
	return cmd.Run()
}

// Push pushes to remote
func (a *Adapter) Push(ctx context.Context, repoPath, remote, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "push", remote, branch)
	cmd.Dir = repoPath
	return cmd.Run()
}

// GetBranches returns list of branches
func (a *Adapter) GetBranches(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	branches := make([]string, 0, len(lines))
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}
		// Remove the * marker for current branch
		branch = strings.TrimPrefix(branch, "* ")
		branches = append(branches, branch)
	}

	return branches, nil
}

// GetCurrentBranch returns current branch name
func (a *Adapter) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// CheckoutBranch switches to branch
func (a *Adapter) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", branch)
	cmd.Dir = repoPath
	return cmd.Run()
}

// IsClean returns true if working tree is clean
func (a *Adapter) IsClean(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) == "", nil
}

// GetStatus returns repository status
func (a *Adapter) GetStatus(ctx context.Context, repoPath string) (usecase.GitStatus, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return usecase.GitStatus{}, err
	}

	var status usecase.GitStatus
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		filePath := line[3:]

		switch {
		case statusCode[0] != ' ' && statusCode[0] != '?':
			// Staged file
			status.StagedFiles = append(status.StagedFiles, filePath)
		case statusCode[0] == '?' && statusCode[1] == '?':
			// Untracked file
			status.UntrackedFiles = append(status.UntrackedFiles, filePath)
		case statusCode[1] != ' ':
			// Modified file
			status.ModifiedFiles = append(status.ModifiedFiles, filePath)
		}
	}

	status.Clean = len(status.StagedFiles)+len(status.ModifiedFiles)+len(status.UntrackedFiles) == 0

	return status, nil
}

// GetLog returns commit log
func (a *Adapter) GetLog(ctx context.Context, repoPath string, limit int) ([]usecase.GitCommit, error) {
	args := []string{"log", "--oneline", "--format=%H|%an|%ad|%s", "--date=iso"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	commits := make([]usecase.GitCommit, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}

		// Parse date
		date, err := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		if err != nil {
			// Fallback to current time if parsing fails
			date = time.Now()
		}

		shortHash := parts[0]
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}

		commit := usecase.GitCommit{
			Hash:      parts[0],
			Author:    parts[1],
			Date:      date,
			Message:   parts[3],
			ShortHash: shortHash,
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// RepoRoot returns repository root path
func (a *Adapter) RepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("not a git repository (no work tree)")
	}
	return strings.TrimSpace(string(output)), nil
}

// ConfigGet reads git config value
func (a *Adapter) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ConfigSet sets git config value.
func (a *Adapter) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", key, value)
	cmd.Dir = repoPath
	return cmd.Run()
}

// ConfigGetGlobal reads global git config value.
func (a *Adapter) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ConfigGetWorktree reads git worktree config value
func (a *Adapter) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--worktree", "--get", key)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ConfigSetWorktree sets git worktree config value
func (a *Adapter) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--worktree", key, value)
	cmd.Dir = repoPath
	return cmd.Run()
}

// ConfigSetGlobal sets global git config value.
func (a *Adapter) ConfigSetGlobal(ctx context.Context, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", key, value)
	return cmd.Run()
}

// GitDir returns git directory for repo
func (a *Adapter) GitDir(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GitCommonDir returns common git directory for repo.
func (a *Adapter) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// WorktreeList returns list of worktrees for repository.
func (a *Adapter) WorktreeList(ctx context.Context, repoPath string) ([]usecase.WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(string(output)), nil
}

func parseWorktreeList(output string) []usecase.WorktreeInfo {
	lines := strings.Split(output, "\n")
	worktrees := make([]usecase.WorktreeInfo, 0)
	var current *usecase.WorktreeInfo
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			current = &usecase.WorktreeInfo{Path: path}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = normalizeWorktreeBranch(ref)
		}
	}
	if current != nil {
		worktrees = append(worktrees, *current)
	}
	return worktrees
}

func normalizeWorktreeBranch(ref string) string {
	trimmed := strings.TrimSpace(ref)
	return strings.TrimPrefix(trimmed, "refs/heads/")
}

// ListIgnoredUntracked returns ignored/untracked files
func (a *Adapter) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}

	raw := output
	parts := strings.Split(string(raw), "\x00")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		results = append(results, part)
	}
	return results, nil
}
