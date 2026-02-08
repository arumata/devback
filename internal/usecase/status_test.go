package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockGitStatus struct {
	repoRootErr error
	gitDirErr   error
	repoRoot    string
	gitDir      string
	commonDir   string
	local       map[string]string
	worktree    map[string]string
	global      map[string]string
	worktrees   []WorktreeInfo
}

func (m *mockGitStatus) RepoRoot(ctx context.Context) (string, error) {
	if m.repoRootErr != nil {
		return "", m.repoRootErr
	}
	return m.repoRoot, nil
}

func (m *mockGitStatus) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	if m.local == nil {
		return "", errors.New("not found")
	}
	value, ok := m.local[key]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (m *mockGitStatus) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	return nil
}

func (m *mockGitStatus) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	if m.global == nil {
		return "", nil
	}
	return m.global[key], nil
}

func (m *mockGitStatus) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	if m.worktree == nil {
		return "", errors.New("not found")
	}
	value, ok := m.worktree[key]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (m *mockGitStatus) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	return nil
}

func (m *mockGitStatus) ConfigSetGlobal(ctx context.Context, key, value string) error {
	return nil
}

func (m *mockGitStatus) GitDir(ctx context.Context, repoPath string) (string, error) {
	if m.gitDirErr != nil {
		return "", m.gitDirErr
	}
	return m.gitDir, nil
}

func (m *mockGitStatus) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	return m.commonDir, nil
}

func (m *mockGitStatus) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	return m.worktrees, nil
}

func (m *mockGitStatus) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	return nil, nil
}

const statusTestSlug = "company/app"

func TestStatus_NoRepo_ConfigMissing(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	homeDir := t.TempDir()
	logger := newStatusLogger()
	fs := newTestFileSystem()
	cfgPort := newFakeConfigPort(fs)
	git := &mockGitStatus{global: map[string]string{}}
	deps := &Dependencies{
		FileSystem: fs,
		Config:     cfgPort,
		Git:        git,
	}

	report, err := Status(ctx, StatusOptions{NoRepo: true, HomeDir: homeDir}, deps, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Repo != nil {
		t.Fatalf("expected no repo info")
	}
	if report.Global.ConfigFile.Exists {
		t.Fatalf("expected missing config file")
	}
	if report.Global.GitTemplateDir.Set {
		t.Fatalf("expected git templateDir to be unset")
	}
	if report.Global.LogDir.Exists {
		t.Fatalf("expected log dir not to exist")
	}
	if report.Global.LogDir.Path != DefaultConfigFile().Logging.Dir {
		t.Fatalf("unexpected log dir path: %s", report.Global.LogDir.Path)
	}
}

func TestStatus_Repo_NoScanBackups(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	env := newStatusRepoEnv(t)
	logger := newStatusLogger()
	seedHooks(t, env.templatesDir, env.hooksDir, true)

	slug := statusTestSlug
	expectedRepoKey, ok := repoKeyFromSlug(env.fs, env.repoRoot, slug)
	if !ok {
		t.Fatalf("expected repo key from slug")
	}

	git := &mockGitStatus{
		repoRoot:  env.repoRoot,
		gitDir:    ".git",
		commonDir: ".git",
		local: map[string]string{
			"backup.slug":    slug,
			"backup.enabled": "true",
		},
		global: map[string]string{
			"init.templateDir": normalizePath(env.fs, env.fs.Dir(DefaultTemplatesDir()), env.homeDir),
		},
		worktrees: []WorktreeInfo{
			{Path: env.repoRoot, Branch: "main"},
		},
	}
	deps := &Dependencies{
		FileSystem: env.fs,
		Config:     env.cfgPort,
		Git:        git,
	}

	report, err := Status(ctx, StatusOptions{HomeDir: env.homeDir}, deps, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Repo == nil {
		t.Fatalf("expected repo info")
	}
	if report.Repo.Type != RepoTypeRegular {
		t.Fatalf("unexpected repo type: %v", report.Repo.Type)
	}
	if report.Repo.Hooks.Installed != len(statusHookFiles()) {
		t.Fatalf("unexpected hooks installed: %d", report.Repo.Hooks.Installed)
	}
	if report.Repo.Hooks.Executable != len(statusHookFiles()) {
		t.Fatalf("unexpected hooks executable: %d", report.Repo.Hooks.Executable)
	}
	if !report.Repo.Hooks.Current.Known || !report.Repo.Hooks.Current.Matches {
		t.Fatalf("expected hooks current to match")
	}
	if !report.Repo.BackupEnabled {
		t.Fatalf("expected backup enabled")
	}
	if report.Repo.BackupSlug != slug {
		t.Fatalf("unexpected backup slug: %s", report.Repo.BackupSlug)
	}
	if report.Repo.RepoKey != expectedRepoKey {
		t.Fatalf("unexpected repo key: %s", report.Repo.RepoKey)
	}
	if report.Repo.Backups.Scanned {
		t.Fatalf("expected backups not scanned")
	}
}

func TestStatus_Worktree_UsesCommonHooksDir(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	env := newStatusWorktreeEnv(t)
	expectedRepoKey, ok := repoKeyFromSlug(env.fs, env.worktreeRoot, env.slug)
	if !ok {
		t.Fatalf("expected repo key from slug")
	}

	repo := mustResolveWorktreeRepo(t, ctx, env.deps)
	assertWorktreeHooks(t, ctx, env.fs, env.git, repo, env.hooksDir)

	report, err := Status(ctx, StatusOptions{HomeDir: env.homeDir}, env.deps, env.logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertWorktreeStatus(t, report, env.slug, expectedRepoKey)
}

type statusWorktreeEnv struct {
	homeDir      string
	worktreeRoot string
	hooksDir     string
	slug         string
	fs           *testFileSystem
	git          *mockGitStatus
	deps         *Dependencies
	logger       *slog.Logger
}

func newStatusWorktreeEnv(t *testing.T) statusWorktreeEnv {
	t.Helper()

	ctx := context.Background()
	homeDir := t.TempDir()
	worktreeRoot := t.TempDir()
	mainRepo := t.TempDir()
	logger := newStatusLogger()
	fs := newTestFileSystem()
	cfgPort := newFakeConfigPort(fs)

	paths := buildInitPaths(fs, homeDir)
	mustMkdirAll(t, filepath.Dir(paths.configPath))
	mustWriteFile(t, paths.configPath, []byte("config"))

	cfg := DefaultConfigFile()
	cfg.Backup.BaseDir = "~/devback-backups"
	cfgPort.data[paths.configPath] = cfg

	templatesDir := normalizePath(fs, DefaultTemplatesDir(), homeDir)
	backupBase := normalizePath(fs, cfg.Backup.BaseDir, homeDir)
	mustMkdirAll(t, templatesDir)
	mustMkdirAll(t, backupBase)

	commonGit := filepath.Join(mainRepo, ".git")
	hooksDir := filepath.Join(commonGit, "hooks")
	gitDir := filepath.Join(commonGit, "worktrees", "feature")
	if !strings.HasPrefix(gitDir, "/") {
		t.Fatalf("expected absolute git dir, got %s", gitDir)
	}
	mustMkdirAll(t, hooksDir)
	mustMkdirAll(t, gitDir)
	seedHooks(t, templatesDir, hooksDir, false)
	mustWriteFile(t, filepath.Join(worktreeRoot, ".git"), []byte("gitdir: "+gitDir+"\n"))

	const slug = statusTestSlug
	git := &mockGitStatus{
		repoRoot:  worktreeRoot,
		gitDir:    gitDir,
		commonDir: ".git",
		worktree: map[string]string{
			"backup.slug":    slug,
			"backup.enabled": "true",
		},
		global: map[string]string{
			"init.templateDir": normalizePath(fs, filepath.Dir(DefaultTemplatesDir()), homeDir),
		},
		worktrees: []WorktreeInfo{
			{Path: mainRepo, Branch: "main"},
			{Path: worktreeRoot, Branch: "feature"},
		},
	}
	deps := &Dependencies{
		FileSystem: fs,
		Config:     cfgPort,
		Git:        git,
	}

	_ = ctx
	return statusWorktreeEnv{
		homeDir:      homeDir,
		worktreeRoot: worktreeRoot,
		hooksDir:     hooksDir,
		slug:         slug,
		fs:           fs,
		git:          git,
		deps:         deps,
		logger:       logger,
	}
}

func mustResolveWorktreeRepo(t *testing.T, ctx context.Context, deps *Dependencies) setupRepo {
	t.Helper()

	repo, err := resolveSetupRepo(ctx, deps, deps.Git.(*mockGitStatus).repoRoot)
	if err != nil {
		t.Fatalf("unexpected resolve repo error: %v", err)
	}
	if !repo.isWorktree {
		t.Fatalf("expected worktree repo")
	}
	if !looksLikeWorktreeGitDir(deps.FileSystem, repo.gitDir) {
		t.Fatalf("unexpected git dir: %s", repo.gitDir)
	}
	return repo
}

func assertWorktreeHooks(
	t *testing.T,
	ctx context.Context,
	fs FileSystemPort,
	git *mockGitStatus,
	repo setupRepo,
	hooksDir string,
) {
	t.Helper()

	installedRepo, err := hooksInstalled(ctx, fs, repo.hooksDir, statusHookFiles())
	if err != nil {
		t.Fatalf("unexpected hooks check error: %v", err)
	}
	installedMain, err := hooksInstalled(ctx, fs, hooksDir, statusHookFiles())
	if err != nil {
		t.Fatalf("unexpected hooks check error: %v", err)
	}
	if installedRepo {
		t.Fatalf("expected hooks missing in worktree dir")
	}
	if !installedMain {
		t.Fatalf("expected hooks installed in common dir")
	}
	resolvedHooks, err := resolveStatusHooksDir(ctx, fs, git, repo, statusHookFiles())
	if err != nil {
		t.Fatalf("unexpected resolve hooks dir error: %v", err)
	}
	if normalizeRepoPath(fs, resolvedHooks) != normalizeRepoPath(fs, hooksDir) {
		t.Fatalf("unexpected resolved hooks dir: %s", resolvedHooks)
	}
}

func assertWorktreeStatus(t *testing.T, report StatusReport, slug, expectedRepoKey string) {
	t.Helper()

	if report.Repo == nil {
		t.Fatalf("expected repo info")
	}
	if report.Repo.Type != RepoTypeWorktree {
		t.Fatalf("unexpected repo type: %v", report.Repo.Type)
	}
	if report.Repo.Hooks.Installed != len(statusHookFiles()) {
		t.Fatalf("unexpected hooks installed: %d", report.Repo.Hooks.Installed)
	}
	if report.Repo.Hooks.Executable != len(statusHookFiles()) {
		t.Fatalf("unexpected hooks executable: %d", report.Repo.Hooks.Executable)
	}
	if !report.Repo.Hooks.Current.Known || !report.Repo.Hooks.Current.Matches {
		t.Fatalf("expected hooks current to match")
	}
	if !report.Repo.BackupEnabled {
		t.Fatalf("expected backup enabled")
	}
	if report.Repo.BackupSlug != slug {
		t.Fatalf("unexpected backup slug: %s", report.Repo.BackupSlug)
	}
	if report.Repo.RepoKey != expectedRepoKey {
		t.Fatalf("unexpected repo key: %s", report.Repo.RepoKey)
	}
}

func TestStatus_ScanBackups(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	env := newStatusRepoEnv(t)
	logger := newStatusLogger()
	seedHooks(t, env.templatesDir, env.hooksDir, false)

	slug := "company/app"
	repoKey, ok := repoKeyFromSlug(env.fs, env.repoRoot, slug)
	if !ok {
		t.Fatalf("expected repo key from slug")
	}

	snapshot1 := createSnapshot(t, env.backupBase, repoKey, "2026-01-02", "120000-000000001", 1024)
	snapshot2 := createSnapshot(t, env.backupBase, repoKey, "2026-01-03", "130000-000000002", 1024)
	firstTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	secondTime := time.Date(2026, 1, 3, 13, 0, 0, 0, time.UTC)
	mustChtimes(t, snapshot1, firstTime, firstTime)
	mustChtimes(t, snapshot2, secondTime, secondTime)

	git := &mockGitStatus{
		repoRoot:  env.repoRoot,
		gitDir:    ".git",
		commonDir: ".git",
		local: map[string]string{
			"backup.slug":    slug,
			"backup.enabled": "true",
		},
		global: map[string]string{
			"init.templateDir": normalizePath(env.fs, env.fs.Dir(DefaultTemplatesDir()), env.homeDir),
		},
	}
	deps := &Dependencies{
		FileSystem: env.fs,
		Config:     env.cfgPort,
		Git:        git,
	}

	report, err := Status(ctx, StatusOptions{HomeDir: env.homeDir, ScanBackups: true}, deps, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Repo == nil {
		t.Fatalf("expected repo info")
	}
	if !report.Repo.Backups.Scanned {
		t.Fatalf("expected backups scanned")
	}
	if report.Repo.Backups.SnapshotCount != 2 {
		t.Fatalf("unexpected snapshot count: %d", report.Repo.Backups.SnapshotCount)
	}
	if report.Repo.Backups.TotalSizeKB != 4 {
		t.Fatalf("unexpected total size: %d", report.Repo.Backups.TotalSizeKB)
	}
	if !report.Repo.Backups.LastBackup.Equal(secondTime) {
		t.Fatalf("unexpected last backup time: %v", report.Repo.Backups.LastBackup)
	}
}

func TestStatus_OutsideGitRepo(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	homeDir := t.TempDir()
	logger := newStatusLogger()
	fs := newTestFileSystem()
	cfgPort := newFakeConfigPort(fs)
	git := &mockGitStatus{
		repoRootErr: errors.New("not a repo"),
		gitDirErr:   errors.New("not a repo"),
	}
	deps := &Dependencies{
		FileSystem: fs,
		Config:     cfgPort,
		Git:        git,
	}

	report, err := Status(ctx, StatusOptions{HomeDir: homeDir}, deps, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Repo != nil {
		t.Fatalf("expected no repo info")
	}
}

func TestFormatStatus_MarksCurrentWorktreeWithNormalizedPaths(t *testing.T) {
	t.Helper()

	report := StatusReport{
		Global: StatusGlobal{
			ConfigFile:   StatusPath{Path: "~/.config/devback/config.toml", Exists: true},
			TemplatesDir: StatusPath{Path: "~/.local/share/devback/templates/hooks", Exists: true, Source: "default"},
			BackupBase:   StatusPath{Path: "~/backup", Exists: true, Source: "from config"},
			LogDir:       StatusPath{Path: "~/.local/state/devback/logs", Exists: true, Source: "default"},
			GitTemplateDir: StatusGitTemplateDir{
				Expected: "~/.local/share/devback/templates",
				Actual:   "~/.local/share/devback/templates",
				Set:      true,
				Matches:  true,
			},
		},
		Repo: &StatusRepo{
			Root:   "/tmp/repo",
			Type:   RepoTypeRegular,
			Branch: "main",
			Hooks: StatusHooks{
				Installed:  3,
				Executable: 3,
				Total:      3,
				Current:    StatusCurrent{Known: true, Matches: true},
			},
			BackupEnabled: true,
			RepoKey:       "repo--hash",
		},
		Worktrees: []WorktreeInfo{
			{Path: "/tmp//repo/./", Branch: "main"},
			{Path: "/tmp/other", Branch: "feature"},
		},
	}

	out := FormatStatus(report, false)

	if !strings.Contains(out, "▶ /tmp//repo/./  [main]") {
		t.Fatalf("expected current worktree marker for normalized-equal path, got:\n%s", out)
	}
	if strings.Contains(out, "▶ /tmp/other  [feature]") {
		t.Fatalf("unexpected current worktree marker for non-current path, got:\n%s", out)
	}
}

func createSnapshot(t *testing.T, backupBase, repoKey, dateDir, timeDir string, size int) string {
	t.Helper()

	snapshotDir := filepath.Join(backupBase, repoKey, dateDir, timeDir)
	mustMkdirAll(t, snapshotDir)
	mustWriteFile(t, filepath.Join(snapshotDir, ".done"), []byte("ok"))
	if size > 0 {
		data := bytes.Repeat([]byte("a"), size)
		mustWriteFile(t, filepath.Join(snapshotDir, "data.bin"), data)
	}
	return snapshotDir
}

type statusRepoEnv struct {
	homeDir      string
	repoRoot     string
	templatesDir string
	backupBase   string
	hooksDir     string
	fs           *testFileSystem
	cfgPort      *fakeConfigPort
	cfg          ConfigFile
}

func newStatusRepoEnv(t *testing.T) statusRepoEnv {
	t.Helper()

	homeDir := t.TempDir()
	repoRoot := t.TempDir()
	fs := newTestFileSystem()
	cfgPort := newFakeConfigPort(fs)

	paths := buildInitPaths(fs, homeDir)
	mustMkdirAll(t, filepath.Dir(paths.configPath))
	mustWriteFile(t, paths.configPath, []byte("config"))

	cfg := DefaultConfigFile()
	cfg.Backup.BaseDir = "~/devback-backups"
	cfgPort.data[paths.configPath] = cfg

	templatesDir := normalizePath(fs, DefaultTemplatesDir(), homeDir)
	backupBase := normalizePath(fs, cfg.Backup.BaseDir, homeDir)
	mustMkdirAll(t, templatesDir)
	mustMkdirAll(t, backupBase)

	hooksDir := filepath.Join(repoRoot, ".git", "hooks")
	mustMkdirAll(t, hooksDir)

	return statusRepoEnv{
		homeDir:      homeDir,
		repoRoot:     repoRoot,
		templatesDir: templatesDir,
		backupBase:   backupBase,
		hooksDir:     hooksDir,
		fs:           fs,
		cfgPort:      cfgPort,
		cfg:          cfg,
	}
}

func seedHooks(t *testing.T, templatesDir, hooksDir string, useCRLF bool) {
	t.Helper()

	for _, name := range statusHookFiles() {
		tmplPath := filepath.Join(templatesDir, name)
		hookPath := filepath.Join(hooksDir, name)
		tmplContent := []byte("echo " + name + "\n")
		hookContent := tmplContent
		if useCRLF && name == "post-commit" {
			tmplContent = []byte("line1\r\nline2\r\n")
			hookContent = []byte("line1\nline2\n")
		}
		mustWriteFile(t, tmplPath, tmplContent)
		mustWriteFile(t, hookPath, hookContent)
		mustChmod(t, hookPath, 0o700)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustChmod(t *testing.T, path string, perm os.FileMode) {
	t.Helper()

	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func mustChtimes(t *testing.T, path string, atime, mtime time.Time) {
	t.Helper()

	if err := os.Chtimes(path, atime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func newStatusLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
