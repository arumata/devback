package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	gitConfigTrue   = "true"
	testDevbackPath = "/tmp/devback"
)

func hookTemplateContent(hookName string) string {
	return "#!/bin/sh\n" +
		"DEVBACK=\"" + testDevbackPath + "\"\n" +
		"[ -x \"$DEVBACK\" ] || { echo \"SKIP: devback not found at $DEVBACK\" >&2; exit 0; }\n" +
		"exec \"$DEVBACK\" hook " + hookName + " \"$@\"\n"
}

type mockGitSetup struct {
	repoRoot string
	gitDir   string
	common   string

	local    map[string]string
	worktree map[string]string
}

func (m *mockGitSetup) RepoRoot(ctx context.Context) (string, error) {
	return m.repoRoot, nil
}

func (m *mockGitSetup) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	if m.local == nil {
		return "", nil
	}
	return m.local[key], nil
}

func (m *mockGitSetup) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	if m.local == nil {
		m.local = make(map[string]string)
	}
	m.local[key] = value
	return nil
}

func (m *mockGitSetup) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *mockGitSetup) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	if m.worktree == nil {
		return "", nil
	}
	return m.worktree[key], nil
}

func (m *mockGitSetup) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	if m.worktree == nil {
		m.worktree = make(map[string]string)
	}
	m.worktree[key] = value
	return nil
}

func (m *mockGitSetup) ConfigSetGlobal(ctx context.Context, key, value string) error {
	return nil
}

func (m *mockGitSetup) GitDir(ctx context.Context, repoPath string) (string, error) {
	return m.gitDir, nil
}

func (m *mockGitSetup) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	return m.common, nil
}

func (m *mockGitSetup) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	return nil, nil
}

func (m *mockGitSetup) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	return nil, nil
}

type setupEnv struct {
	homeDir  string
	repoRoot string
	gitDir   string
	deps     *Dependencies
	git      *mockGitSetup
}

func newSetupEnv(t *testing.T) setupEnv {
	t.Helper()

	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "post-commit"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	repoTemplatesDir := expandHomeDir(DefaultRepoTemplatesDir(), homeDir)
	if err := os.MkdirAll(repoTemplatesDir, 0o750); err != nil {
		t.Fatalf("mkdir repo templates dir: %v", err)
	}
	devbackignorePath := filepath.Join(repoTemplatesDir, "devbackignore")
	if err := os.WriteFile(devbackignorePath, []byte("# test\nnode_modules\n"), 0o600); err != nil {
		t.Fatalf("write repo template: %v", err)
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	return setupEnv{
		homeDir:  homeDir,
		repoRoot: repoRoot,
		gitDir:   gitDir,
		deps:     deps,
		git:      git,
	}
}

func TestSetup_NormalRepo_InstallsHooksAndConfig(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	files := map[string]string{
		"post-commit": "hook",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(templatesDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	for name := range files {
		if _, err := os.Stat(filepath.Join(hooksDir, name)); err != nil {
			t.Fatalf("expected hook %s to exist: %v", name, err)
		}
	}
	if got := git.local["backup.enabled"]; got != gitConfigTrue {
		t.Fatalf("expected backup.enabled to be true, got %q", got)
	}
}

func TestSetup_NormalRepo_SetsExecutableModes(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	for _, name := range statusHookFiles() {
		if err := os.WriteFile(filepath.Join(templatesDir, name), []byte("data"), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	for _, name := range statusHookFiles() {
		info, err := os.Stat(filepath.Join(hooksDir, name))
		if err != nil {
			t.Fatalf("stat hook %s: %v", name, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected hook %s to be executable", name)
		}
	}
}

func TestSetup_NormalRepo_ExistingHooks_NoForce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "post-commit"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing hook: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(templatesDir, "post-commit"),
		[]byte(hookTemplateContent("post-commit")),
		0o600,
	); err != nil {
		t.Fatalf("write template: %v", err)
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	backupPath := filepath.Join(hooksDir, "post-commit.devback.orig")
	// #nosec G304 -- test paths are controlled by the test harness.
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read hook backup: %v", err)
	}
	if string(backupData) != "old" {
		t.Fatalf("expected backup to contain original hook, got %q", string(backupData))
	}
	// #nosec G304 -- test paths are controlled by the test harness.
	mergedData, err := os.ReadFile(filepath.Join(hooksDir, "post-commit"))
	if err != nil {
		t.Fatalf("read merged hook: %v", err)
	}
	if !strings.Contains(string(mergedData), devbackMergedHookMarker) {
		t.Fatalf("expected merged hook to contain marker, got %q", string(mergedData))
	}
}

func TestSetup_NormalRepo_ExistingHooks_Ours_NoBackup(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(hooksDir, "post-commit"),
		[]byte(hookTemplateContent("post-commit")),
		0o600,
	); err != nil {
		t.Fatalf("write existing hook: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(templatesDir, "post-commit"),
		[]byte(hookTemplateContent("post-commit")),
		0o600,
	); err != nil {
		t.Fatalf("write template: %v", err)
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(hooksDir, "post-commit.devback.orig")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no backup file, got %v", err)
	}
}

func TestSetup_NormalRepo_Force_Overwrites(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "post-commit"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing hook: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "post-commit"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir, Force: true}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// #nosec G304 -- test paths are controlled by the test harness.
	data, err := os.ReadFile(filepath.Join(hooksDir, "post-commit"))
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("expected hook to be overwritten, got %q", string(data))
	}
	if got := git.local["backup.enabled"]; got != gitConfigTrue {
		t.Fatalf("expected backup.enabled to be true, got %q", got)
	}
}

func TestSetup_NoHooksOrDryRun_NoChanges(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name string
		opts SetupOptions
	}{
		{
			name: "no-hooks",
			opts: SetupOptions{NoHooks: true},
		},
		{
			name: "dry-run",
			opts: SetupOptions{DryRun: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newSetupEnv(t)
			tc.opts.HomeDir = env.homeDir

			if err := Setup(ctx, tc.opts, env.deps, logger); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if _, err := os.Stat(filepath.Join(env.gitDir, "hooks", "post-commit")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected hook not to be created, got %v", err)
			}
			if _, ok := env.git.local["backup.enabled"]; ok {
				t.Fatalf("expected backup.enabled not to be set")
			}
		})
	}
}

func TestSetup_Worktree_ConfiguresSlugAndEnabled(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git", "worktrees", "feature")
	commonDir := filepath.Join(repoRoot, ".git-main")
	hooksDir := filepath.Join(commonDir, "hooks")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	content := "hook"
	for _, name := range setupRequiredFiles() {
		if err := os.WriteFile(filepath.Join(templatesDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write hook %s: %v", name, err)
		}
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git/worktrees/feature",
		common:   ".git-main",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	opts := SetupOptions{HomeDir: homeDir, Slug: "owner/project"}
	if err := Setup(ctx, opts, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := git.local["backup.enabled"]; got != gitConfigTrue {
		t.Fatalf("expected backup.enabled to be true, got %q", got)
	}
	if got := git.local["extensions.worktreeConfig"]; got != gitConfigTrue {
		t.Fatalf("expected extensions.worktreeConfig to be true, got %q", got)
	}
	if got := git.worktree["backup.slug"]; got != "owner/project" {
		t.Fatalf("expected worktree backup.slug to be set, got %q", got)
	}
}

func TestSetup_Worktree_ForceRejected(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git", "worktrees", "feature")
	commonDir := filepath.Join(repoRoot, ".git-main")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.MkdirAll(commonDir, 0o750); err != nil {
		t.Fatalf("mkdir common dir: %v", err)
	}

	fs := newTestFileSystem()

	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git/worktrees/feature",
		common:   ".git-main",
	}

	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	err := Setup(ctx, SetupOptions{HomeDir: homeDir, Force: true}, deps, logger)
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestSetup_DevbackIgnore_Created(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := newSetupEnv(t)

	if err := Setup(ctx, SetupOptions{HomeDir: env.homeDir}, env.deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	devbackIgnorePath := filepath.Join(env.repoRoot, ".devbackignore")
	// #nosec G304 -- test paths are controlled by the test harness.
	data, err := os.ReadFile(devbackIgnorePath)
	if err != nil {
		t.Fatalf("expected .devbackignore to be created: %v", err)
	}
	if !strings.Contains(string(data), "node_modules") {
		t.Fatal("expected .devbackignore to contain template content")
	}

	templatePath := filepath.Join(expandHomeDir(DefaultRepoTemplatesDir(), env.homeDir), "devbackignore")
	templateInfo, err := os.Stat(templatePath)
	if err != nil {
		t.Fatalf("stat repo template: %v", err)
	}
	destInfo, err := os.Stat(devbackIgnorePath)
	if err != nil {
		t.Fatalf("stat .devbackignore: %v", err)
	}
	if templateInfo.Mode().Perm() != destInfo.Mode().Perm() {
		t.Fatalf("expected .devbackignore mode %o, got %o", templateInfo.Mode().Perm(), destInfo.Mode().Perm())
	}
}

func TestSetup_DevbackIgnore_NotOverwritten(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := newSetupEnv(t)

	existingContent := "# my custom devbackignore\n"
	devbackIgnorePath := filepath.Join(env.repoRoot, ".devbackignore")
	if err := os.WriteFile(devbackIgnorePath, []byte(existingContent), 0o600); err != nil {
		t.Fatalf("write existing .devbackignore: %v", err)
	}

	if err := Setup(ctx, SetupOptions{HomeDir: env.homeDir}, env.deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// #nosec G304 -- test paths are controlled by the test harness.
	data, err := os.ReadFile(devbackIgnorePath)
	if err != nil {
		t.Fatalf("read .devbackignore: %v", err)
	}
	if string(data) != existingContent {
		t.Fatalf("expected .devbackignore to remain unchanged, got %q", string(data))
	}
}

func TestSetup_DevbackIgnore_NotCreatedInWorktree(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git", "worktrees", "feature")
	commonDir := filepath.Join(repoRoot, ".git-main")
	hooksDir := filepath.Join(commonDir, "hooks")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	for _, name := range setupRequiredFiles() {
		if err := os.WriteFile(filepath.Join(templatesDir, name), []byte("hook"), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(hooksDir, name), []byte("hook"), 0o600); err != nil {
			t.Fatalf("write hook %s: %v", name, err)
		}
	}

	repoTemplatesDir := expandHomeDir(DefaultRepoTemplatesDir(), homeDir)
	if err := os.MkdirAll(repoTemplatesDir, 0o750); err != nil {
		t.Fatalf("mkdir repo templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoTemplatesDir, "devbackignore"), []byte("# test\n"), 0o600); err != nil {
		t.Fatalf("write repo template: %v", err)
	}

	fs := newTestFileSystem()
	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git/worktrees/feature",
		common:   ".git-main",
	}
	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	devbackIgnorePath := filepath.Join(repoRoot, ".devbackignore")
	if _, err := os.Stat(devbackIgnorePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected .devbackignore NOT to be created in worktree")
	}
}

func TestSetup_DevbackIgnore_NotCreatedOnDryRun(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := newSetupEnv(t)

	if err := Setup(ctx, SetupOptions{HomeDir: env.homeDir, DryRun: true}, env.deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	devbackIgnorePath := filepath.Join(env.repoRoot, ".devbackignore")
	if _, err := os.Stat(devbackIgnorePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected .devbackignore NOT to be created on dry-run")
	}
}

func TestSetup_DevbackIgnore_MissingTemplate(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	repoRoot := t.TempDir()

	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}

	templatesDir := expandHomeDir(DefaultTemplatesDir(), homeDir)
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "post-commit"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	// intentionally do NOT create repo templates dir

	fs := newTestFileSystem()
	git := &mockGitSetup{
		repoRoot: repoRoot,
		gitDir:   ".git",
		common:   ".git",
	}
	deps := &Dependencies{
		FileSystem: fs,
		Git:        git,
	}

	if err := Setup(ctx, SetupOptions{HomeDir: homeDir}, deps, logger); err != nil {
		t.Fatalf("expected no error even without repo template, got %v", err)
	}

	devbackIgnorePath := filepath.Join(repoRoot, ".devbackignore")
	if _, err := os.Stat(devbackIgnorePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected .devbackignore NOT to be created when template missing")
	}
}
