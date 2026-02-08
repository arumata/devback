package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const gitDirName = ".git"

func newTestBackupContext(verbose bool) *backupContext {
	return newBackupContext(slog.Default(), verbose)
}

func deriveRepoKeyForTest(t *testing.T, repoRoot string, cfg *Config, mock *mockGit) string {
	t.Helper()
	deps := &Dependencies{Git: mock, FileSystem: newTestFileSystem()}
	return deriveRepoKey(context.Background(), cfg, deps, repoRoot, newTestBackupContext(cfg.Verbose))
}

type failingReadFS struct {
	*testFileSystem
}

func (f failingReadFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if strings.HasSuffix(path, ".devbackignore") {
		return nil, fmt.Errorf("read failed")
	}
	return f.testFileSystem.ReadFile(ctx, path)
}

type failingDoneFS struct {
	*testFileSystem
}

func (f failingDoneFS) WriteFile(ctx context.Context, path string, data []byte, perm int) error {
	if strings.HasSuffix(path, ".done") {
		return fmt.Errorf("write failed")
	}
	return f.testFileSystem.WriteFile(ctx, path, data, perm)
}

func TestResolveRepoRoot_UsesGit(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoPath); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
		Git:        newTestGitAdapter(),
	}

	root, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != repoPath {
		t.Fatalf("expected repo root %s, got %s", repoPath, root)
	}
}

func TestResolveRepoRoot_FallbackToWorkingDir(t *testing.T) {
	ctx := context.Background()
	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != wd {
		t.Fatalf("expected working dir %s, got %s", wd, root)
	}
}

func TestEnsureGitRepo_AllowsGitFile(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	actualGitDir := filepath.Join(tempDir, "actual-git-dir")
	if err := os.MkdirAll(actualGitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFilePath := filepath.Join(repoRoot, gitDirName)
	if err := os.WriteFile(gitFilePath, []byte("gitdir: ../actual-git-dir\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
		Git: &mockGit{
			GitDirFunc: func(ctx context.Context, repoPath string) (string, error) {
				return "../actual-git-dir", nil
			},
		},
	}
	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		t.Fatalf("expected .git file to be accepted, got %v", err)
	}
}

func TestEnsureGitRepo_InvalidGitFile(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFilePath := filepath.Join(repoRoot, gitDirName)
	if err := os.WriteFile(gitFilePath, []byte("not-a-gitdir\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
		Git: &mockGit{
			GitDirFunc: func(ctx context.Context, repoPath string) (string, error) {
				return "", fmt.Errorf("not a git repo")
			},
		},
	}
	if err := ensureGitRepo(ctx, deps, repoRoot); err == nil {
		t.Fatal("expected error for invalid .git file")
	}
}

func TestEnsureGitRepo_RelativeGitDirInSubdir(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repoRoot, "worktrees", "feature")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFilePath := filepath.Join(repoRoot, gitDirName)
	if err := os.WriteFile(gitFilePath, []byte("gitdir: worktrees/feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
		Git: &mockGit{
			GitDirFunc: func(ctx context.Context, repoPath string) (string, error) {
				return "worktrees/feature", nil
			},
		},
	}
	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		t.Fatalf("expected relative gitdir in subdir to be accepted, got %v", err)
	}
}

func TestDeriveRepoKey_AutoSlug(t *testing.T) {
	ctx := context.Background()
	repoRoot := filepath.Join("/tmp", "my repo")
	cfg := &Config{RepoKeyStyle: repoKeyStyleAuto, Verbose: false}
	mock := &mockGit{
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if key == "backup.slug" {
				return "work/acme", nil
			}
			return "", os.ErrNotExist
		},
		GitDirFunc: func(ctx context.Context, repoPath string) (string, error) {
			return gitDirName, nil
		},
	}
	deps := &Dependencies{Git: mock, FileSystem: newTestFileSystem()}

	key := deriveRepoKey(ctx, cfg, deps, repoRoot, newTestBackupContext(cfg.Verbose))

	if key != "work/acme/my_repo" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestDeriveRepoKey_AutoRemoteWithHash(t *testing.T) {
	ctx := context.Background()
	repoRoot := filepath.Join("/tmp", "repo")
	cfg := &Config{RepoKeyStyle: repoKeyStyleAuto, AutoRemoteMerge: false, RemoteHashLen: 8}
	mock := &mockGit{
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if key == "remote.origin.url" {
				return "git@github.com:acme/app.git", nil
			}
			return "", os.ErrNotExist
		},
	}
	deps := &Dependencies{Git: mock, FileSystem: newTestFileSystem()}

	key := deriveRepoKey(ctx, cfg, deps, repoRoot, newTestBackupContext(cfg.Verbose))

	expectedPrefix := "github.com/acme/app--"
	if !strings.HasPrefix(key, expectedPrefix) {
		t.Fatalf("expected prefix %s, got %s", expectedPrefix, key)
	}
	if len(key) != len(expectedPrefix)+8 {
		t.Fatalf("expected hash length 8, got %s", key)
	}
}

func TestDeriveRepoKey_CustomFallback(t *testing.T) {
	ctx := context.Background()
	repoRoot := filepath.Join("/tmp", "Repo")
	cfg := &Config{RepoKeyStyle: repoKeyStyleCustom}
	mock := &mockGit{}
	deps := &Dependencies{Git: mock, FileSystem: newTestFileSystem()}

	key := deriveRepoKey(ctx, cfg, deps, repoRoot, newTestBackupContext(cfg.Verbose))
	if !strings.HasPrefix(key, "Repo--") {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestDeriveRepoKey_CustomSlug(t *testing.T) {
	repoRoot := filepath.Join("/tmp", "Repo")
	cfg := &Config{RepoKeyStyle: repoKeyStyleCustom}
	mock := &mockGit{
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if key == "backup.slug" {
				return "work/acme", nil
			}
			return "", os.ErrNotExist
		},
	}
	key := deriveRepoKeyForTest(t, repoRoot, cfg, mock)
	if key != "work/acme/Repo" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestDeriveRepoKey_RemoteHierarchy(t *testing.T) {
	repoRoot := filepath.Join("/tmp", "repo")
	cfg := &Config{RepoKeyStyle: repoKeyStyleRemoteHierarchy}
	mock := &mockGit{
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if key == "remote.origin.url" {
				return "https://github.com/acme/app.git", nil
			}
			return "", os.ErrNotExist
		},
	}
	key := deriveRepoKeyForTest(t, repoRoot, cfg, mock)
	if key != "github.com/acme/app" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestParseRemoteVariants(t *testing.T) {
	host, owner, repo := parseRemote("git@github.com:acme/app.git")
	if host != "github.com" || owner != "acme" || repo != "app" {
		t.Fatalf("unexpected scp parse: %s %s %s", host, owner, repo)
	}

	host, owner, repo = parseRemote("https://github.com/acme/app.git")
	if host != "github.com" || owner != "acme" || repo != "app" {
		t.Fatalf("unexpected https parse: %s %s %s", host, owner, repo)
	}

	host, owner, repo = parseRemote("invalid")
	if host != "" || owner != "" || repo != "" {
		t.Fatalf("unexpected parse for invalid remote: %s %s %s", host, owner, repo)
	}
}

func TestShortHashN_Bounds(t *testing.T) {
	if got := shortHashN("data", -1); len(got) != 8 {
		t.Fatalf("expected default length 8, got %d", len(got))
	}
	if got := shortHashN("data", 80); len(got) != 64 {
		t.Fatalf("expected max length 64, got %d", len(got))
	}
}

func TestReadDevbackIgnoreAndShouldSkip(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoRoot := t.TempDir()
	ignorePath := filepath.Join(repoRoot, ".devbackignore")
	content := "# comment\nbuild/\nfoo/bar\n*.tmp\nlogs/*.log\n"
	if err := os.WriteFile(ignorePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	excludes, err := readDevbackIgnore(ctx, &Dependencies{FileSystem: fs}, repoRoot, newTestBackupContext(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excludes) != 4 {
		t.Fatalf("expected 4 excludes, got %d", len(excludes))
	}
	if skip, ex := shouldSkip("build/output.bin", excludes); !skip || ex != "build" {
		t.Fatalf("expected build to be skipped, got %v %s", skip, ex)
	}
	if skip, ex := shouldSkip("notes.tmp", excludes); !skip || ex != "*.tmp" {
		t.Fatalf("expected *.tmp to be skipped, got %v %s", skip, ex)
	}
	if skip, ex := shouldSkip("logs/app.log", excludes); !skip || ex != "logs/*.log" {
		t.Fatalf("expected logs/*.log to be skipped, got %v %s", skip, ex)
	}
}

func TestIsPermissionError(t *testing.T) {
	fs := newTestFileSystem()
	if !isPermissionError(fs, os.ErrPermission) {
		t.Fatal("expected permission error to be detected")
	}
	if !isPermissionError(fs, syscall.EPERM) {
		t.Fatal("expected EPERM to be detected")
	}
	if isPermissionError(fs, os.ErrNotExist) {
		t.Fatal("expected non-permission error to be ignored")
	}
}

func TestCopyDirRecursive_CopiesSymlink(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	src := t.TempDir()
	dst := t.TempDir()

	subDir := filepath.Join(src, "dir")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(src, "link.txt")
	if err := os.Symlink(filePath, linkPath); err != nil {
		t.Fatal(err)
	}

	result := &BackupResult{}
	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: fs},
		src,
		dst,
		result,
		newTestBackupContext(false),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	copiedFile := filepath.Join(dst, "dir", "file.txt")
	if _, err := os.Stat(copiedFile); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}

	copiedLink := filepath.Join(dst, "link.txt")
	info, err := os.Lstat(copiedLink)
	if err != nil {
		t.Fatalf("expected copied symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink, got mode %v", info.Mode())
	}
}

func TestCopyDirRecursive_PermissionError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			return walkFn(filepath.Join(root, "file.txt"), nil, os.ErrPermission)
		},
	}
	result := &BackupResult{}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		result,
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
	if !result.PartialSuccess {
		t.Fatal("expected partial success")
	}
	if result.SkippedFiles != 1 {
		t.Fatalf("expected 1 skipped file, got %d", result.SkippedFiles)
	}
}

func TestCopyDirRecursive_CreateDirError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			info := &mockFileInfo{isDir: true, mode: 0o750}
			return walkFn(filepath.Join(root, "dir"), info, nil)
		},
		CreateDirFunc: func(ctx context.Context, path string, perm int) error {
			return os.ErrPermission
		},
	}
	result := &BackupResult{}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		result,
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
	if !result.PartialSuccess {
		t.Fatal("expected partial success")
	}
}

func TestCopyDirRecursive_CopyError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			info := &mockFileInfo{isDir: false, mode: 0o600}
			return walkFn(filepath.Join(root, "file.txt"), info, nil)
		},
		CopyFunc: func(ctx context.Context, src, dst string) error {
			return fmt.Errorf("copy failed")
		},
	}
	result := &BackupResult{}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		result,
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestCopyDirRecursive_SymlinkReadlinkError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			info := &mockFileInfo{isDir: false, mode: int(os.ModeSymlink)}
			return walkFn(filepath.Join(root, "link"), info, nil)
		},
		ReadlinkFunc: func(ctx context.Context, path string) (string, error) {
			return "", os.ErrPermission
		},
	}
	result := &BackupResult{}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		result,
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
	if !result.PartialSuccess {
		t.Fatal("expected partial success")
	}
}

func TestCopyDirRecursive_SymlinkCreateError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			info := &mockFileInfo{isDir: false, mode: int(os.ModeSymlink)}
			return walkFn(filepath.Join(root, "link"), info, nil)
		},
		ReadlinkFunc: func(ctx context.Context, path string) (string, error) {
			return "/target", nil
		},
		SymlinkFunc: func(ctx context.Context, target, path string) error {
			return os.ErrPermission
		},
	}
	result := &BackupResult{}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		result,
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
	if !result.PartialSuccess {
		t.Fatal("expected partial success")
	}
}

func TestCopyDirRecursive_InfoNil(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		WalkFunc: func(ctx context.Context, root string, walkFn WalkFunc) error {
			return walkFn(filepath.Join(root, "nil"), nil, nil)
		},
	}

	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: mockFS},
		"/src",
		"/dst",
		&BackupResult{},
		newTestBackupContext(false),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopySelectedFiles(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "dir"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "file.txt"), filepath.Join(src, "link.txt")); err != nil {
		t.Fatal(err)
	}

	paths := []string{"file.txt", "dir", "link.txt"}
	if err := copySelectedFiles(
		ctx,
		&Dependencies{FileSystem: fs},
		paths,
		src,
		dst,
		&BackupResult{},
		newTestBackupContext(false),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "dir")); err != nil {
		t.Fatalf("expected copied dir: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(dst, "link.txt")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected copied symlink")
	}
}

func TestCopySelectedFiles_LstatError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		LstatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}
	if err := copySelectedFiles(
		ctx,
		&Dependencies{FileSystem: mockFS},
		[]string{"missing.txt"},
		"/src",
		"/dst",
		&BackupResult{},
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestCopySelectedFiles_ReadlinkError(t *testing.T) {
	ctx := context.Background()
	mockFS := &mockFileSystem{
		LstatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return &mockFileInfo{mode: int(os.ModeSymlink)}, nil
		},
		ReadlinkFunc: func(ctx context.Context, path string) (string, error) {
			return "", os.ErrPermission
		},
	}
	if err := copySelectedFiles(
		ctx,
		&Dependencies{FileSystem: mockFS},
		[]string{"link"},
		"/src",
		"/dst",
		&BackupResult{},
		newTestBackupContext(false),
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestListSnapshotsAndRotateRepo(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoDir := t.TempDir()

	snap1 := filepath.Join(repoDir, "2023-01-01", "000000")
	snap2 := filepath.Join(repoDir, "2023-01-02", "000000")
	if err := os.MkdirAll(snap1, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(snap2, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap1, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap2, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(snap1, ".done"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	snaps, err := listSnapshots(ctx, &Dependencies{FileSystem: fs}, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	cfg := &Config{KeepDays: 1, KeepCount: 1}
	rotateRepo(ctx, &Dependencies{FileSystem: fs}, repoDir, cfg, false, newTestBackupContext(false))

	remaining, err := listSnapshots(ctx, &Dependencies{FileSystem: fs}, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(remaining))
	}
}

func TestRotateRepo_SizeDryRunAndSummary(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoDir := t.TempDir()

	snap := filepath.Join(repoDir, "2023-02-01", "000000")
	if err := os.MkdirAll(snap, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "file.bin"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{MaxTotalGBPerRepo: 1, SizeMarginMB: -1024, NoSize: false}
	rotateRepo(ctx, &Dependencies{FileSystem: fs}, repoDir, cfg, true, newTestBackupContext(true))
}

func TestRemoveSnapshot_DoesNotRemoveDateDirWithRemaining(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoDir := t.TempDir()

	dateDir := filepath.Join(repoDir, "2024-03-01")
	snap1 := filepath.Join(dateDir, "010101")
	snap2 := filepath.Join(dateDir, "020202")
	if err := os.MkdirAll(snap1, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(snap2, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap1, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap2, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	removeSnapshot(ctx, &Dependencies{FileSystem: fs}, snapshot{
		DateDir: dateDir,
		TimeDir: snap1,
		Done:    filepath.Join(snap1, ".done"),
	}, newTestBackupContext(false))

	if _, err := os.Stat(dateDir); err != nil {
		t.Fatalf("expected date dir to remain: %v", err)
	}
	if _, err := os.Stat(snap2); err != nil {
		t.Fatalf("expected other snapshot to remain: %v", err)
	}
}

func TestHandleBackupFlow_Success(t *testing.T) {
	ctx := context.Background()
	repoRoot := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repoRoot, "test.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "ignored.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	backupDir := t.TempDir()
	repoKey := "repo--deadbeef"
	repoDir := filepath.Join(backupDir, repoKey)
	if err := os.MkdirAll(repoDir, 0o750); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		BackupDir: backupDir,
		NoSize:    true,
	}
	deps := &Dependencies{
		FileSystem: newTestFileSystem(),
		Git:        newTestGitAdapter(),
	}

	if _, err := handleBackupFlow(
		ctx,
		cfg,
		deps,
		repoRoot,
		repoDir,
		newTestBackupContext(cfg.Verbose),
	); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".done")); err == nil {
		t.Fatal("unexpected .done at repo root")
	}
}

func TestHandleBackupFlow_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := handleBackupFlow(
		ctx,
		&Config{},
		&Dependencies{},
		"/repo",
		"/repoDir",
		newTestBackupContext(false),
	)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected interrupted error, got %v", err)
	}
}

func TestHandleBackupFlow_DoneWriteError(t *testing.T) {
	ctx := context.Background()
	repoRoot := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repoRoot, "test.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	backupDir := t.TempDir()
	repoKey := "repo--deadbeef"
	repoDir := filepath.Join(backupDir, repoKey)
	if err := os.MkdirAll(repoDir, 0o750); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		BackupDir: backupDir,
		NoSize:    true,
	}
	deps := &Dependencies{
		FileSystem: failingDoneFS{testFileSystem: newTestFileSystem()},
		Git:        newTestGitAdapter(),
	}

	_, err := handleBackupFlow(
		ctx,
		cfg,
		deps,
		repoRoot,
		repoDir,
		newTestBackupContext(cfg.Verbose),
	)
	if !errors.Is(err, ErrCritical) {
		t.Fatalf("expected critical error, got %v", err)
	}

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		t.Fatalf("read repo dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatal("expected cleanup after failed backup")
	}
}

func TestHandleDryRun_NoFilesystemWrites(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{DryRun: true, BackupDir: "/backup"}
	mockFS := &mockFileSystem{
		CreateDirFunc: func(ctx context.Context, path string, perm int) error {
			t.Fatalf("unexpected CreateDir in dry-run: %s", path)
			return nil
		},
		WriteFileFunc: func(ctx context.Context, path string, data []byte, perm int) error {
			t.Fatalf("unexpected WriteFile in dry-run: %s", path)
			return nil
		},
		RemoveAllFunc: func(ctx context.Context, path string) error {
			t.Fatalf("unexpected RemoveAll in dry-run: %s", path)
			return nil
		},
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			if strings.HasSuffix(path, gitDirName) {
				return &mockFileInfo{isDir: true}, nil
			}
			if strings.Contains(path, "/backup/") {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{isDir: true}, nil
		},
		ReadFileFunc: func(ctx context.Context, path string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
	}
	mockGit := &mockGit{
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return []string{"ignored.txt"}, nil
		},
	}
	deps := &Dependencies{FileSystem: mockFS, Git: mockGit}

	if _, err := handleDryRun(ctx, cfg, deps, "/repo", "repo", newTestBackupContext(cfg.Verbose)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestHandleBackupFlow_CopyError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{BackupDir: "/backup"}
	mockFS := &mockFileSystem{
		CreateDirFunc: func(ctx context.Context, path string, perm int) error {
			return nil
		},
		WriteFileFunc: func(ctx context.Context, path string, data []byte, perm int) error {
			return fmt.Errorf("write failed")
		},
	}

	_, err := handleBackupFlow(
		ctx,
		cfg,
		&Dependencies{FileSystem: mockFS},
		"/repo",
		"/repoDir",
		newTestBackupContext(cfg.Verbose),
	)
	if !errors.Is(err, ErrCritical) {
		t.Fatalf("expected critical error, got %v", err)
	}
}

func TestCopyRepoSnapshot_MissingGit(t *testing.T) {
	ctx := context.Background()
	err := copyRepoSnapshot(ctx, &Dependencies{}, "/repo", "/dst", &BackupResult{}, newTestBackupContext(false))
	if err == nil {
		t.Fatal("expected error without git adapter")
	}
}

func TestCopyRepoSnapshot_NoGitDir(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	deps := &Dependencies{FileSystem: fs, Git: newTestGitAdapter()}
	repoRoot := t.TempDir()
	target := t.TempDir()

	err := copyRepoSnapshot(ctx, deps, repoRoot, target, &BackupResult{}, newTestBackupContext(false))
	if err == nil {
		t.Fatal("expected error when .git is missing")
	}
}

func TestCopyRepoSnapshot_UsesCommonGitDir(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoRoot := t.TempDir()
	commonRoot := t.TempDir()
	commonGit := filepath.Join(commonRoot, "common-git")
	if err := os.MkdirAll(commonGit, 0o750); err != nil {
		t.Fatal(err)
	}
	headPath := filepath.Join(commonGit, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktreeGitDir := filepath.Join(commonGit, "worktrees", "devback-wrk")
	if err := os.MkdirAll(worktreeGitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	gitFilePath := filepath.Join(repoRoot, gitDirName)
	if err := os.WriteFile(gitFilePath, []byte("gitdir: "+worktreeGitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &mockGit{
		GitDirFunc: func(ctx context.Context, repoPath string) (string, error) {
			return worktreeGitDir, nil
		},
		GitCommonDirFunc: func(ctx context.Context, repoPath string) (string, error) {
			return commonGit, nil
		},
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return nil, nil
		},
	}
	deps := &Dependencies{FileSystem: fs, Git: mock}
	target := t.TempDir()

	if err := copyRepoSnapshot(ctx, deps, repoRoot, target, &BackupResult{}, newTestBackupContext(false)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(target, gitDirName))
	if err != nil {
		t.Fatalf("stat backup .git: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected backup .git to be a directory")
	}
	data, err := os.ReadFile(filepath.Join(target, gitDirName, "HEAD")) // #nosec G304 - test data
	if err != nil {
		t.Fatalf("read backup HEAD: %v", err)
	}
	if string(data) != "ref: refs/heads/main\n" {
		t.Fatalf("unexpected HEAD content: %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(target, gitDirName, "worktrees")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected worktrees dir to be removed, got %v", err)
	}
}

func TestCopyRepoSnapshot_ListIgnoredError(t *testing.T) {
	ctx := context.Background()
	fs := newTestFileSystem()
	repoRoot := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, gitDirName), 0o750); err != nil {
		t.Fatal(err)
	}

	mock := &mockGit{
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return nil, fmt.Errorf("git error")
		},
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			return "", os.ErrNotExist
		},
	}

	deps := &Dependencies{FileSystem: fs, Git: mock}
	err := copyRepoSnapshot(ctx, deps, repoRoot, target, &BackupResult{}, newTestBackupContext(false))
	if !errors.Is(err, ErrCritical) {
		t.Fatalf("expected critical error, got %v", err)
	}
}

func TestCopyRepoSnapshot_DevbackIgnoreError(t *testing.T) {
	ctx := context.Background()
	repoRoot := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, gitDirName), 0o750); err != nil {
		t.Fatal(err)
	}

	mock := &mockGit{
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return []string{}, nil
		},
	}

	deps := &Dependencies{FileSystem: failingReadFS{testFileSystem: newTestFileSystem()}, Git: mock}
	err := copyRepoSnapshot(ctx, deps, repoRoot, target, &BackupResult{}, newTestBackupContext(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleDryRun_ListIgnoredError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{DryRun: true, BackupDir: "/backup"}
	mockFS := &mockFileSystem{
		StatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			if strings.HasSuffix(path, gitDirName) {
				return &mockFileInfo{isDir: true}, nil
			}
			if strings.Contains(path, "/backup/") {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{isDir: true}, nil
		},
		ReadFileFunc: func(ctx context.Context, path string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
	}
	mockGit := &mockGit{
		ListIgnoredUntrackedFunc: func(ctx context.Context, repoPath string) ([]string, error) {
			return nil, fmt.Errorf("git error")
		},
	}
	deps := &Dependencies{FileSystem: mockFS, Git: mockGit}

	_, err := handleDryRun(ctx, cfg, deps, "/repo", "repo", newTestBackupContext(cfg.Verbose))
	if !errors.Is(err, ErrCritical) {
		t.Fatalf("expected critical error, got %v", err)
	}
}

func TestHumanKB(t *testing.T) {
	if got := humanKB(10); got != "10 KiB" {
		t.Fatalf("unexpected value: %s", got)
	}
	if got := humanKB(2048); !strings.Contains(got, "MiB") {
		t.Fatalf("expected MiB formatting, got %s", got)
	}
	if got := humanKB(1024 * 1024); !strings.Contains(got, "GiB") {
		t.Fatalf("expected GiB formatting, got %s", got)
	}
}

func TestPrintBackupSummary(t *testing.T) {
	result := &BackupResult{
		SkippedFiles:   2,
		PermissionErrs: []string{"permission denied: /a", "permission denied: /b"},
		PartialSuccess: true,
	}
	printBackupSummary(result, newTestBackupContext(true))
}

func TestMatchDateTimeDir(t *testing.T) {
	if !matchDateDir("2024-01-01") {
		t.Fatal("expected date to match")
	}
	if matchDateDir("20240101") {
		t.Fatal("expected date to not match")
	}
	if !matchTimeDir("235959") {
		t.Fatal("expected time to match")
	}
	if !matchTimeDir("235959-000000123") {
		t.Fatal("expected time with nanos to match")
	}
	if !matchTimeDir("235959-000000123-01") {
		t.Fatal("expected time with suffix to match")
	}
	if matchTimeDir("24abcd") {
		t.Fatal("expected time to not match")
	}
}
