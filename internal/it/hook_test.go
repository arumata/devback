package it

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookPostCommit_BackupCreated(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)
	commitFile(t, env, repoPath, "post-commit.txt", "post commit", "post-commit")

	runHook(t, env, repoPath, []string{"hook", "post-commit"})

	if got := countSnapshots(t, backupBase, repoKey); got != 1 {
		t.Fatalf("expected 1 snapshot, got %d", got)
	}
}

func TestHookPostCommit_RebaseInProgress(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)

	rebaseDir := filepath.Join(repoPath, ".git", "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0o750); err != nil {
		t.Fatal(err)
	}

	runHook(t, env, repoPath, []string{"hook", "post-commit"})

	if got := countSnapshots(t, backupBase, repoKey); got != 0 {
		t.Fatalf("expected 0 snapshots, got %d", got)
	}
}

func TestHookPostCommit_Disabled(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)
	setBackupEnabled(t, env, repoPath, "false")

	runHook(t, env, repoPath, []string{"hook", "post-commit"})

	if got := countSnapshots(t, backupBase, repoKey); got != 0 {
		t.Fatalf("expected 0 snapshots, got %d", got)
	}
}

func TestHookPostMerge_BackupCreated(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)
	baseBranch := currentBranch(t, env, repoPath)

	runGit(t, env, repoPath, "checkout", "-b", "feature")
	commitFile(t, env, repoPath, "feature.txt", "feature", "feature")
	resetToBranch(t, env, repoPath, baseBranch)
	runGit(t, env, repoPath, "merge", "feature")

	runHook(t, env, repoPath, []string{"hook", "post-merge"})

	if got := countSnapshots(t, backupBase, repoKey); got != 1 {
		t.Fatalf("expected 1 snapshot, got %d", got)
	}
}

func TestHookPostRewrite_RebaseDebounce(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)

	runHook(t, env, repoPath, []string{"hook", "post-rewrite", "rebase"})
	if got := countSnapshots(t, backupBase, repoKey); got != 1 {
		t.Fatalf("expected 1 snapshot after first hook, got %d", got)
	}

	runHook(t, env, repoPath, []string{"hook", "post-rewrite", "rebase"})
	if got := countSnapshots(t, backupBase, repoKey); got != 1 {
		t.Fatalf("expected debounce to keep 1 snapshot, got %d", got)
	}
}

func TestHookPostRewrite_Amend(t *testing.T) {
	env, repoPath, backupBase, repoKey := setupHookEnv(t)
	commitFile(t, env, repoPath, "amend.txt", "amend", "amend")
	runGit(t, env, repoPath, "commit", "--amend", "-m", "amend")

	runHook(t, env, repoPath, []string{"hook", "post-rewrite", "amend"})

	if got := countSnapshots(t, backupBase, repoKey); got != 1 {
		t.Fatalf("expected 1 snapshot, got %d", got)
	}
}

func setupHookEnv(t *testing.T) (TestEnv, string, string, string) {
	t.Helper()
	buildBinary(t)
	env := newTestEnv(t)
	repoPath := setupTestRepo(t, env)
	runGit(t, env, repoPath, "config", "user.email", "test@test.com")
	runGit(t, env, repoPath, "config", "user.name", "Test User")
	runDevback(t, env, env.TempDir, []string{"init", "--backup-dir", "~/backup"})

	backupBase := expandHomeDir("~/backup", env.HomeDir)
	if err := os.MkdirAll(backupBase, 0o750); err != nil {
		t.Fatal(err)
	}
	setBackupEnabled(t, env, repoPath, "true")

	repoKey := runDevbackOutput(t, env, repoPath, []string{"--print-repo-key"})
	if strings.TrimSpace(repoKey) == "" {
		t.Fatal("expected repo key")
	}

	return env, repoPath, backupBase, repoKey
}

func runHook(t *testing.T, env TestEnv, repoPath string, args []string) {
	t.Helper()
	cmd := exec.Command(getBinaryPath(t), args...)
	cmd.Dir = repoPath
	cmd.Env = buildCommandEnv(TestCase{}, env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("devback %v failed: %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
}

func setBackupEnabled(t *testing.T, env TestEnv, repoPath, value string) {
	t.Helper()
	runGit(t, env, repoPath, "config", "backup.enabled", value)
}

func commitFile(t *testing.T, env TestEnv, repoPath, name, content, message string) {
	t.Helper()
	path := filepath.Join(repoPath, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, env, repoPath, "add", name)
	runGit(t, env, repoPath,
		"-c", "user.email=test@test.com",
		"-c", "user.name=Test User",
		"commit", "-m", message,
	)
}

func currentBranch(t *testing.T, env TestEnv, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	cmd.Env = gitEnv(env)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current failed: %v", err)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		t.Fatal("expected current branch")
	}
	return branch
}

func resetToBranch(t *testing.T, env TestEnv, repoPath, branch string) {
	t.Helper()
	runGit(t, env, repoPath, "checkout", branch)
}

func runGit(t *testing.T, env TestEnv, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Env = gitEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
}

func countSnapshots(t *testing.T, backupBase, repoKey string) int {
	t.Helper()
	if strings.TrimSpace(repoKey) == "" {
		t.Fatal("repo key is empty")
	}
	repoDir := filepath.Join(backupBase, repoKey)
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}

	count := 0
	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if info.Name() == ".done" {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}
