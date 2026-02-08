package it

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	configadapter "github.com/arumata/devback/internal/adapters/config"
	templatesadapter "github.com/arumata/devback/internal/adapters/templates"
	"github.com/arumata/devback/internal/usecase"
)

//nolint:gochecknoglobals // flag is required at package scope for go test.
var updateGolden = flag.Bool("update", false, "update golden files")

func getUpdateFlag() bool {
	if updateGolden != nil && *updateGolden {
		return true
	}
	value := strings.TrimSpace(os.Getenv("DEVBACK_UPDATE_GOLDEN"))
	return value == "1" || strings.EqualFold(value, "true")
}

func buildBinary(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Build the new architecture binary (without legacy tag)
	cmd := exec.Command("go", "build", "-o", "../../bin/devback-test", ".")
	cmd.Dir = filepath.Join(wd, "../../cmd/app")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}
}

type TestEnv struct {
	TempDir string
	HomeDir string
}

type TestCase struct {
	Name        string
	Args        []string
	ExpectError bool
	Setup       func(t *testing.T, env TestEnv) string // returns repo path
	Cleanup     func(t *testing.T, env TestEnv)
	Env         map[string]string
	FSTreeRoots []string // relative to env.TempDir unless absolute
}

type TestResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	FSTree   []string
}

func TestE2E(t *testing.T) {
	// Build the binary first
	buildBinary(t)

	testCases := []TestCase{
		{
			Name:        "help",
			Args:        []string{"--help"},
			ExpectError: false,
		},
		{
			Name:        "init_help",
			Args:        []string{"init", "--help"},
			ExpectError: false,
		},
		{
			Name:        "setup_help",
			Args:        []string{"setup", "--help"},
			ExpectError: false,
		},
		{
			Name:        "status_help",
			Args:        []string{"status", "--help"},
			ExpectError: false,
		},
		{
			Name:        "print_repo_key",
			Args:        []string{"--print-repo-key"},
			ExpectError: false,
			Setup:       setupTestRepo,
		},
		{
			Name:        "backup_simple",
			Args:        []string{},
			ExpectError: false,
			Setup:       setupTestRepoForBackup,
			FSTreeRoots: []string{"backup"},
		},
		{
			Name:        "backup_verbose",
			Args:        []string{"-v"},
			ExpectError: false,
			Setup:       setupTestRepoWithConfig,
			FSTreeRoots: []string{"backup"},
		},
		{
			Name:        "backup_dry_run",
			Args:        []string{"--dry-run"},
			ExpectError: false,
			Setup:       setupTestRepoForBackup,
			FSTreeRoots: []string{"backup"},
		},
		{
			Name:        "test_locks",
			Args:        []string{"--test-locks"},
			ExpectError: false,
		},
		{
			Name:        "missing_backup_dir",
			Args:        []string{},
			ExpectError: true,
		},
		{
			Name:        "invalid_repo",
			Args:        []string{},
			ExpectError: true,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupInvalidRepo(t, env)
				cfg := usecase.DefaultConfigFile()
				cfg.Backup.BaseDir = filepath.Join(env.TempDir, "backup")
				writeConfig(t, env, cfg)
				return repoPath
			},
		},
		{
			Name:        "init_success",
			Args:        []string{"init", "--backup-dir", "~/backup"},
			ExpectError: false,
			FSTreeRoots: []string{
				"home/.config/devback",
				"home/.local/share/devback/templates/hooks",
				"home/.local/share/devback/repo-templates",
				"home/.local/state/devback/logs",
				"home/backup",
			},
		},
		{
			Name:        "init_dry_run",
			Args:        []string{"init", "--dry-run", "--backup-dir", "~/backup"},
			ExpectError: false,
		},
		{
			Name:        "init_existing_config",
			Args:        []string{"init"},
			ExpectError: true,
			Setup: func(t *testing.T, env TestEnv) string {
				writeConfig(t, env, usecase.DefaultConfigFile())
				return ""
			},
		},
		{
			Name:        "init_force",
			Args:        []string{"init", "--force", "--backup-dir", "~/backup"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				writeConfig(t, env, usecase.DefaultConfigFile())
				return ""
			},
			FSTreeRoots: []string{
				"home/.config/devback",
				"home/.local/share/devback/templates/hooks",
				"home/.local/share/devback/repo-templates",
				"home/.local/state/devback/logs",
				"home/backup",
			},
		},
		{
			Name:        "init_templates_only",
			Args:        []string{"init", "--templates-only"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				writeConfig(t, env, usecase.DefaultConfigFile())
				return ""
			},
			FSTreeRoots: []string{
				"home/.local/share/devback/templates/hooks",
				"home/.local/share/devback/repo-templates",
			},
		},
		{
			Name:        "setup_success",
			Args:        []string{"setup"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				templatesDir := expandHomeDir(usecase.DefaultTemplatesDir(), env.HomeDir)
				installTemplates(t, templatesDir)
				installRepoTemplates(t, env.HomeDir)
				return repoPath
			},
			FSTreeRoots: []string{"test-repo/.git/hooks"},
		},
		{
			Name:        "setup_dry_run",
			Args:        []string{"setup", "--dry-run"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				templatesDir := expandHomeDir(usecase.DefaultTemplatesDir(), env.HomeDir)
				installTemplates(t, templatesDir)
				installRepoTemplates(t, env.HomeDir)
				return repoPath
			},
		},
		{
			Name:        "setup_force",
			Args:        []string{"setup", "--force"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				templatesDir := expandHomeDir(usecase.DefaultTemplatesDir(), env.HomeDir)
				installTemplates(t, templatesDir)
				installRepoTemplates(t, env.HomeDir)
				hooksDir := filepath.Join(repoPath, ".git", "hooks")
				if err := os.MkdirAll(hooksDir, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(hooksDir, "post-commit"), []byte("conflict"), 0o600); err != nil {
					t.Fatal(err)
				}
				return repoPath
			},
			FSTreeRoots: []string{"test-repo/.git/hooks"},
		},
		{
			Name:        "setup_worktree",
			Args:        []string{"setup", "--no-hooks"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				mainRepo := setupTestRepo(t, env)
				worktreePath := filepath.Join(env.TempDir, "worktree")
				cmd := exec.Command("git", "worktree", "add", worktreePath)
				cmd.Dir = mainRepo
				cmd.Env = gitEnv(env)
				if err := cmd.Run(); err != nil {
					t.Fatal(err)
				}
				templatesDir := expandHomeDir(usecase.DefaultTemplatesDir(), env.HomeDir)
				installTemplates(t, templatesDir)
				installRepoTemplates(t, env.HomeDir)
				commonDirCmd := exec.Command("git", "rev-parse", "--git-common-dir")
				commonDirCmd.Dir = worktreePath
				commonDirCmd.Env = gitEnv(env)
				commonDirRaw, err := commonDirCmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				commonDir := strings.TrimSpace(string(commonDirRaw))
				if !filepath.IsAbs(commonDir) {
					commonDir = filepath.Join(worktreePath, commonDir)
				}
				installHooksFromTemplates(t, templatesDir, filepath.Join(commonDir, "hooks"))
				return worktreePath
			},
		},
		{
			Name:        "setup_no_hooks",
			Args:        []string{"setup", "--no-hooks"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				templatesDir := expandHomeDir(usecase.DefaultTemplatesDir(), env.HomeDir)
				installTemplates(t, templatesDir)
				installRepoTemplates(t, env.HomeDir)
				return repoPath
			},
			FSTreeRoots: []string{"test-repo/.git/hooks"},
		},
		{
			Name:        "setup_not_a_repo",
			Args:        []string{"setup"},
			ExpectError: true,
			Setup:       setupInvalidRepo,
		},
		{
			Name:        "status_configured",
			Args:        []string{"status"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				runDevback(t, env, env.TempDir, []string{"init", "--backup-dir", "~/backup"})
				runDevback(t, env, repoPath, []string{"setup"})
				return repoPath
			},
		},
		{
			Name:        "status_unconfigured",
			Args:        []string{"status"},
			ExpectError: false,
			Setup:       setupTestRepo,
		},
		{
			Name:        "status_no_repo",
			Args:        []string{"status"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				runDevback(t, env, env.TempDir, []string{"init", "--backup-dir", "~/backup"})
				noRepoDir := filepath.Join(env.TempDir, "no-repo")
				if err := os.MkdirAll(noRepoDir, 0o750); err != nil {
					t.Fatal(err)
				}
				return noRepoDir
			},
		},
		{
			Name:        "status_scan_backups",
			Args:        []string{"status", "--scan-backups"},
			ExpectError: false,
			Setup: func(t *testing.T, env TestEnv) string {
				repoPath := setupTestRepo(t, env)
				runDevback(t, env, env.TempDir, []string{"init", "--backup-dir", "~/backup"})
				runDevback(t, env, repoPath, []string{"setup"})
				backupBase := expandHomeDir("~/backup", env.HomeDir)
				repoKey := runDevbackOutput(t, env, repoPath, []string{"--print-repo-key"})
				firstTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
				secondTime := time.Date(2026, 1, 3, 13, 0, 0, 0, time.UTC)
				createSnapshot(t, backupBase, repoKey, "2026-01-02", "120000-000000001", 1024, firstTime)
				createSnapshot(t, backupBase, repoKey, "2026-01-03", "130000-000000002", 1024, secondTime)
				return repoPath
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			runE2ETest(t, tc)
		})
	}
}

func runE2ETest(t *testing.T, tc TestCase) {
	env := newTestEnv(t)
	repoPath := setupTestCase(t, tc, env)
	args := buildArgs(tc)
	cmd := createCommand(t, tc, args, env, repoPath)
	result := executeCommand(cmd)

	if len(tc.FSTreeRoots) > 0 {
		roots := resolveFSTreeRoots(env, tc.FSTreeRoots)
		result.FSTree = collectFSTree(t, env.TempDir, roots)
	}

	checkExpectations(t, tc, result)
	checkGolden(t, tc.Name, result)
}

func setupTestCase(t *testing.T, tc TestCase, env TestEnv) string {
	var repoPath string
	if tc.Setup != nil {
		repoPath = tc.Setup(t, env)
		if tc.Cleanup != nil {
			defer tc.Cleanup(t, env)
		}
	}
	return repoPath
}

func buildArgs(tc TestCase) []string {
	return append([]string{}, tc.Args...)
}

func createCommand(t *testing.T, tc TestCase, args []string, env TestEnv, repoPath string) *exec.Cmd {
	// Get absolute path to test binary
	binaryPath := getBinaryPath(t)

	// For repo-specific tests, change to repo directory
	var cmd *exec.Cmd
	if repoPath != "" && !contains(tc.Args, "--test-locks") && !contains(tc.Args, "--help") {
		cmd = exec.Command(binaryPath, args...)
		cmd.Dir = repoPath
	} else {
		cmd = exec.Command(binaryPath, args...)
		cmd.Dir = env.TempDir
	}

	cmd.Env = buildCommandEnv(tc, env)
	return cmd
}

func executeCommand(cmd *exec.Cmd) TestResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return TestResult{
		ExitCode: exitCode,
		Stdout:   normalizeOutput(stdout.String()),
		Stderr:   normalizeOutput(stderr.String()),
	}
}

func newTestEnv(t *testing.T) TestEnv {
	t.Helper()

	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	return TestEnv{
		TempDir: tempDir,
		HomeDir: homeDir,
	}
}

func getBinaryPath(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "../../bin/devback-test")
}

func buildCommandEnv(tc TestCase, env TestEnv) []string {
	result := map[string]string{}
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = parts[1]
	}

	result["TZ"] = "UTC"
	result["HOME"] = env.HomeDir
	result["GIT_CONFIG_GLOBAL"] = filepath.Join(env.HomeDir, ".gitconfig")

	for key, value := range tc.Env {
		result[key] = value
	}

	envList := make([]string, 0, len(result))
	for key, value := range result {
		envList = append(envList, key+"="+value)
	}
	return envList
}

func gitEnv(env TestEnv, extra ...string) []string {
	result := append(os.Environ(),
		"HOME="+env.HomeDir,
		"GIT_CONFIG_GLOBAL="+filepath.Join(env.HomeDir, ".gitconfig"),
	)
	result = append(result, extra...)
	return result
}

func resolveFSTreeRoots(env TestEnv, roots []string) []string {
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		clean := strings.TrimSpace(root)
		if clean == "" {
			continue
		}
		if strings.HasPrefix(clean, "~") {
			clean = expandHomeDir(clean, env.HomeDir)
		}
		if !filepath.IsAbs(clean) {
			clean = filepath.Join(env.TempDir, clean)
		}
		resolved = append(resolved, clean)
	}
	return resolved
}

func checkExpectations(t *testing.T, tc TestCase, result TestResult) {
	if tc.ExpectError && result.ExitCode == 0 {
		t.Errorf("Expected error but command succeeded")
	}
	if !tc.ExpectError && result.ExitCode != 0 {
		t.Errorf("Expected success but got exit code %d. Stderr: %s", result.ExitCode, result.Stderr)
	}
}

func setupTestRepo(t *testing.T, env TestEnv) string {
	repoPath := filepath.Join(env.TempDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatal(err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	cmd.Env = gitEnv(env)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Create some test files
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create ignored file
	gitignore := filepath.Join(repoPath, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ignoredFile := filepath.Join(repoPath, "ignored.txt")
	if err := os.WriteFile(ignoredFile, []byte("ignored content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Add and commit
	cmd = exec.Command("git", "add", "test.txt", ".gitignore")
	cmd.Dir = repoPath
	cmd.Env = gitEnv(env)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command("git",
		"-c", "user.email=test@test.com",
		"-c", "user.name=Test User",
		"commit", "-m", "initial commit")
	cmd.Dir = repoPath
	cmd.Env = gitEnv(env,
		"GIT_COMMITTER_DATE=2025-01-01T12:00:00Z",
		"GIT_AUTHOR_DATE=2025-01-01T12:00:00Z",
	)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	return repoPath
}

func setupTestRepoForBackup(t *testing.T, env TestEnv) string {
	repoPath := setupTestRepo(t, env)
	cfg := usecase.DefaultConfigFile()
	cfg.Backup.BaseDir = filepath.Join(env.TempDir, "backup")
	writeConfig(t, env, cfg)
	return repoPath
}

func setupTestRepoWithConfig(t *testing.T, env TestEnv) string {
	repoPath := setupTestRepo(t, env)
	cfg := usecase.DefaultConfigFile()
	cfg.Backup.BaseDir = filepath.Join(env.TempDir, "backup")
	cfg.Backup.KeepCount = 2
	cfg.Backup.KeepDays = 7
	cfg.Backup.MaxTotalGB = 1
	cfg.RepoKey.Style = "name+hash"
	writeConfig(t, env, cfg)
	return repoPath
}

func setupInvalidRepo(t *testing.T, env TestEnv) string {
	repoPath := filepath.Join(env.TempDir, "invalid-repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatal(err)
	}
	// Not a git repo - should cause error
	return repoPath
}

func writeConfig(t *testing.T, env TestEnv, cfg usecase.ConfigFile) {
	t.Helper()

	configDir := filepath.Join(env.HomeDir, ".config", "devback")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	adapter := configadapter.New(newDiscardLogger())
	if err := adapter.Save(context.Background(), configPath, cfg); err != nil {
		t.Fatal(err)
	}
}

func installTemplates(t *testing.T, templatesDir string) {
	t.Helper()

	binPath := getBinaryPath(t)
	adapter := templatesadapter.New(newDiscardLogger())
	entries, err := adapter.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(templatesDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := adapter.Read(context.Background(), entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte(usecase.HookBinaryPlaceholder), []byte(binPath))
		dest := filepath.Join(templatesDir, entry.Name)
		mode := safeFileMode(entry.Mode)
		if err := os.WriteFile(dest, data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dest, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func installRepoTemplates(t *testing.T, homeDir string) {
	t.Helper()

	adapter := templatesadapter.New(newDiscardLogger())
	entries, err := adapter.ListRepo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	repoTemplatesDir := expandHomeDir(usecase.DefaultRepoTemplatesDir(), homeDir)
	if err := os.MkdirAll(repoTemplatesDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := adapter.ReadRepo(context.Background(), entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(repoTemplatesDir, entry.Name)
		mode := safeFileMode(entry.Mode)
		if err := os.WriteFile(dest, data, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func installHooksFromTemplates(t *testing.T, templatesDir, hooksDir string) {
	t.Helper()

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(templatesDir, entry.Name())
		dst := filepath.Join(hooksDir, entry.Name())
		data, err := os.ReadFile(src) // #nosec G304 - test data under temp dir.
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dst, info.Mode()); err != nil {
			t.Fatal(err)
		}
	}
}

func createSnapshot(t *testing.T, backupBase, repoKey, dateDir, timeDir string, size int, mtime time.Time) {
	t.Helper()

	snapshotDir := filepath.Join(backupBase, repoKey, dateDir, timeDir)
	if err := os.MkdirAll(snapshotDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, ".done"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if size > 0 {
		data := bytes.Repeat([]byte("a"), size)
		if err := os.WriteFile(filepath.Join(snapshotDir, "data.bin"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(snapshotDir, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func expandHomeDir(path, homeDir string) string {
	clean := strings.TrimSpace(path)
	if clean == "~" {
		return homeDir
	}
	if strings.HasPrefix(clean, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(clean, "~/"))
	}
	return clean
}

func runDevback(t *testing.T, env TestEnv, dir string, args []string) {
	t.Helper()

	cmd := exec.Command(getBinaryPath(t), args...)
	cmd.Dir = dir
	cmd.Env = buildCommandEnv(TestCase{}, env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("devback %v failed: %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
}

func runDevbackOutput(t *testing.T, env TestEnv, dir string, args []string) string {
	t.Helper()

	cmd := exec.Command(getBinaryPath(t), args...)
	cmd.Dir = dir
	cmd.Env = buildCommandEnv(TestCase{}, env)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("devback %v failed: %v\n%s", args, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		t.Fatalf("devback %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

func safeFileMode(mode int) os.FileMode {
	if mode < 0 || mode > 0o777 {
		return 0o644
	}
	return os.FileMode(mode)
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func normalizeOutput(output string) string {
	// Normalize line endings
	output = strings.ReplaceAll(output, "\r\n", "\n")

	// Replace temp paths with $TMPDIR - more comprehensive patterns
	re := regexp.MustCompile(`/var/folders/[^/\s]+/[^/\s]+/[^/\s]+/[^/\s]+`)
	output = re.ReplaceAllString(output, "$$TMPDIR")

	re = regexp.MustCompile(`/tmp/[^/\s]+`)
	output = re.ReplaceAllString(output, "$$TMPDIR")

	// Replace TestE2E test names in temp paths
	re = regexp.MustCompile(`TestE2E[a-zA-Z_]+[A-Z]+[0-9]+`)
	output = re.ReplaceAllString(output, "TestE2EXXXXX")

	// Replace compact log timestamps (HH:MM:SS at line start)
	re = regexp.MustCompile(`(?m)^\d{2}:\d{2}:\d{2} `)
	output = re.ReplaceAllString(output, "TIMESTAMP ")

	// Replace PIDs in lock test output (PID: 12345)
	re = regexp.MustCompile(`PID: \d+`)
	output = re.ReplaceAllString(output, "PID: XXXXX")

	// Replace PIDs in structured logs (pid=12345)
	re = regexp.MustCompile(`pid=\d+`)
	output = re.ReplaceAllString(output, "pid=HHMMSS")

	// Replace timestamps - be more specific to avoid Git objects
	re = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	output = re.ReplaceAllString(output, "YYYY-MM-DD")

	re = regexp.MustCompile(`\b\d{8}-\d{6}\b`)
	output = re.ReplaceAllString(output, "YYYYMMDD-HHMMSS")

	re = regexp.MustCompile(`\b\d{6}(?:-\d+)+\b`)
	output = re.ReplaceAllString(output, "HHMMSS-NANO")

	re = regexp.MustCompile(`\b\d{6}\b`)
	output = re.ReplaceAllString(output, "HHMMSS")

	// Replace hash suffixes
	re = regexp.MustCompile(`--[a-f0-9]{8}`)
	output = re.ReplaceAllString(output, "--HASH")

	re = regexp.MustCompile(`\[(main|master)\]`)
	output = re.ReplaceAllString(output, "[BRANCH]")

	return strings.TrimSpace(output)
}

func collectFSTree(t *testing.T, baseDir string, roots []string) []string {
	files := make([]string, 0)
	seen := make(map[string]struct{})

	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(baseDir, path)
			if err != nil {
				return err
			}

			// Normalize relative path
			relPath = normalizeOutput(relPath)

			if info.IsDir() {
				relPath += "/"
			}
			if _, ok := seen[relPath]; ok {
				return nil
			}
			seen[relPath] = struct{}{}
			files = append(files, relPath)
			return nil
		})
		if err != nil {
			t.Logf("Warning: could not walk path %s: %v", root, err)
		}
	}

	sort.Strings(files)
	return files
}

func checkGolden(t *testing.T, testName string, result TestResult) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	goldenDir := filepath.Join(wd, "../../testdata/golden", testName)

	if getUpdateFlag() {
		// Create golden files
		if err := os.MkdirAll(goldenDir, 0o750); err != nil {
			t.Fatal(err)
		}

		// Write exit code
		exitCodeFile := filepath.Join(goldenDir, "exit_code.txt")
		if err := os.WriteFile(exitCodeFile, []byte(fmt.Sprintf("%d\n", result.ExitCode)), 0o600); err != nil {
			t.Fatal(err)
		}

		// Write stdout
		if result.Stdout != "" {
			stdoutFile := filepath.Join(goldenDir, "stdout.golden")
			if err := os.WriteFile(stdoutFile, []byte(result.Stdout+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		// Write stderr
		if result.Stderr != "" {
			stderrFile := filepath.Join(goldenDir, "stderr.golden")
			if err := os.WriteFile(stderrFile, []byte(result.Stderr+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		// Write fs tree
		if len(result.FSTree) > 0 {
			fsTreeFile := filepath.Join(goldenDir, "fs_tree.golden")
			content := strings.Join(result.FSTree, "\n") + "\n"
			if err := os.WriteFile(fsTreeFile, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		return
	}

	// Compare with golden files
	compareGoldenFile(t, filepath.Join(goldenDir, "exit_code.txt"), fmt.Sprintf("%d", result.ExitCode))

	if result.Stdout != "" {
		compareGoldenFile(t, filepath.Join(goldenDir, "stdout.golden"), result.Stdout)
	}

	if result.Stderr != "" {
		compareGoldenFile(t, filepath.Join(goldenDir, "stderr.golden"), result.Stderr)
	}

	if len(result.FSTree) > 0 {
		expected := strings.Join(result.FSTree, "\n")
		compareGoldenFile(t, filepath.Join(goldenDir, "fs_tree.golden"), expected)
	}
}

func compareGoldenFile(t *testing.T, goldenPath, actual string) {
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		t.Errorf("Golden file %s does not exist. Run with -update to create it.", goldenPath)
		return
	}

	// Validate path is within testdata directory for security
	absPath, err := filepath.Abs(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testdataPath := filepath.Join(wd, "../../testdata")
	absTestdataPath, err := filepath.Abs(testdataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(absPath, absTestdataPath) {
		t.Fatalf("Golden file path %s is outside testdata directory", goldenPath)
	}

	cleanPath := filepath.Clean(goldenPath)
	expected, err := os.ReadFile(cleanPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedStr := strings.TrimSpace(string(expected))
	actualStr := strings.TrimSpace(actual)

	if expectedStr != actualStr {
		t.Errorf("Golden file %s mismatch.\nExpected:\n%s\n\nActual:\n%s", goldenPath, expectedStr, actualStr)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
