package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type testFileSystem struct{}

func newTestFileSystem() *testFileSystem {
	return &testFileSystem{}
}

func safeFileMode(perm int, fallback fs.FileMode) fs.FileMode {
	if perm < 0 || perm > 0o777 {
		return fallback
	}
	// #nosec G115 -- perm validated to be within safe range.
	return fs.FileMode(perm)
}

func (a *testFileSystem) ReadFile(ctx context.Context, path string) ([]byte, error) {
	_ = ctx
	// #nosec G304 -- test paths are controlled by the test harness.
	return os.ReadFile(path)
}

func (a *testFileSystem) WriteFile(ctx context.Context, path string, data []byte, perm int) error {
	_ = ctx
	return os.WriteFile(path, data, safeFileMode(perm, 0o644))
}

func (a *testFileSystem) CreateDir(ctx context.Context, path string, perm int) error {
	_ = ctx
	return os.MkdirAll(path, safeFileMode(perm, 0o755))
}

func (a *testFileSystem) RemoveAll(ctx context.Context, path string) error {
	_ = ctx
	return os.RemoveAll(path)
}

func (a *testFileSystem) Stat(ctx context.Context, path string) (FileInfo, error) {
	_ = ctx
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &fileInfoWrapperTest{info}, nil
}

func (a *testFileSystem) Lstat(ctx context.Context, path string) (FileInfo, error) {
	_ = ctx
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &fileInfoWrapperTest{info}, nil
}

func (a *testFileSystem) Walk(ctx context.Context, root string, walkFn WalkFunc) error {
	_ = ctx
	return filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		var fileInfo FileInfo
		if info != nil {
			fileInfo = &fileInfoWrapperTest{info}
		}
		return walkFn(path, fileInfo, err)
	})
}

func (a *testFileSystem) ReadDir(ctx context.Context, path string) ([]DirEntry, error) {
	_ = ctx
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, &dirEntryWrapperTest{entry})
	}
	return result, nil
}

func (a *testFileSystem) Glob(ctx context.Context, pattern string) ([]string, error) {
	_ = ctx
	return filepath.Glob(pattern)
}

func (a *testFileSystem) CreateDirExclusive(ctx context.Context, path string, perm int) error {
	_ = ctx
	return os.Mkdir(path, safeFileMode(perm, 0o755))
}

func (a *testFileSystem) Copy(ctx context.Context, src, dst string) error {
	_ = ctx
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	// #nosec G304 -- test paths are controlled by the test harness.
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcFile.Close()
	}()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// #nosec G304 -- test paths are controlled by the test harness.
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = dstFile.Close()
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

func (a *testFileSystem) Move(ctx context.Context, src, dst string) error {
	_ = ctx
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (a *testFileSystem) Readlink(ctx context.Context, path string) (string, error) {
	_ = ctx
	return os.Readlink(path)
}

func (a *testFileSystem) Symlink(ctx context.Context, target, path string) error {
	_ = ctx
	return os.Symlink(target, path)
}

func (a *testFileSystem) Chmod(ctx context.Context, path string, perm int) error {
	_ = ctx
	if perm < 0 || perm > 0o777 {
		return os.ErrInvalid
	}
	return os.Chmod(path, fs.FileMode(perm))
}

func (a *testFileSystem) Chtimes(ctx context.Context, path string, atime, mtime time.Time) error {
	_ = ctx
	return os.Chtimes(path, atime, mtime)
}

func (a *testFileSystem) GetWorkingDir(ctx context.Context) (string, error) {
	_ = ctx
	return os.Getwd()
}

func (a *testFileSystem) Abs(ctx context.Context, path string) (string, error) {
	_ = ctx
	return filepath.Abs(path)
}

func (a *testFileSystem) Join(elements ...string) string {
	return filepath.Join(elements...)
}

func (a *testFileSystem) Base(path string) string {
	return filepath.Base(path)
}

func (a *testFileSystem) Dir(path string) string {
	return filepath.Dir(path)
}

func (a *testFileSystem) Ext(path string) string {
	return filepath.Ext(path)
}

func (a *testFileSystem) TempDir(ctx context.Context, dir, prefix string) (string, error) {
	_ = ctx
	return os.MkdirTemp(dir, prefix)
}

func (a *testFileSystem) IsAbs(path string) bool { return filepath.IsAbs(path) }
func (a *testFileSystem) Rel(basepath, targpath string) (string, error) {
	return filepath.Rel(basepath, targpath)
}
func (a *testFileSystem) Clean(path string) string      { return filepath.Clean(path) }
func (a *testFileSystem) VolumeName(path string) string { return filepath.VolumeName(path) }
func (a *testFileSystem) PathSeparator() byte           { return os.PathSeparator }
func (a *testFileSystem) IsNotExist(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR)
}
func (a *testFileSystem) IsExist(err error) bool { return os.IsExist(err) }
func (a *testFileSystem) IsPermission(err error) bool {
	return os.IsPermission(err) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

type fileInfoWrapperTest struct {
	info fs.FileInfo
}

func (f *fileInfoWrapperTest) Name() string       { return f.info.Name() }
func (f *fileInfoWrapperTest) Size() int64        { return f.info.Size() }
func (f *fileInfoWrapperTest) Mode() int          { return int(f.info.Mode()) }
func (f *fileInfoWrapperTest) ModTime() time.Time { return f.info.ModTime() }
func (f *fileInfoWrapperTest) IsDir() bool        { return f.info.IsDir() }
func (f *fileInfoWrapperTest) IsSymlink() bool    { return f.info.Mode()&os.ModeSymlink != 0 }
func (f *fileInfoWrapperTest) IsRegular() bool    { return f.info.Mode().IsRegular() }
func (f *fileInfoWrapperTest) Sys() interface{}   { return f.info.Sys() }

type dirEntryWrapperTest struct {
	entry fs.DirEntry
}

func (d *dirEntryWrapperTest) Name() string { return d.entry.Name() }
func (d *dirEntryWrapperTest) IsDir() bool  { return d.entry.IsDir() }

type testGitAdapter struct{}

func newTestGitAdapter() *testGitAdapter {
	return &testGitAdapter{}
}

func (a *testGitAdapter) RepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (a *testGitAdapter) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (a *testGitAdapter) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", key, value)
	cmd.Dir = repoPath
	return cmd.Run()
}

func (a *testGitAdapter) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (a *testGitAdapter) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--worktree", "--get", key)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (a *testGitAdapter) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--worktree", key, value)
	cmd.Dir = repoPath
	return cmd.Run()
}

func (a *testGitAdapter) ConfigSetGlobal(ctx context.Context, key, value string) error {
	return fmt.Errorf("not implemented")
}

func (a *testGitAdapter) GitDir(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (a *testGitAdapter) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (a *testGitAdapter) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseWorktreeListOutput(string(output)), nil
}

func (a *testGitAdapter) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(output), "\x00")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		results = append(results, part)
	}
	return results, nil
}

func parseWorktreeListOutput(output string) []WorktreeInfo {
	lines := strings.Split(output, "\n")
	worktrees := make([]WorktreeInfo, 0)
	var current *WorktreeInfo
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
			current = &WorktreeInfo{Path: path}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	if current != nil {
		worktrees = append(worktrees, *current)
	}
	return worktrees
}
