package git

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupRepo(t *testing.T, adapter *Adapter, dir string) {
	ctx := context.Background()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := adapter.Add(ctx, dir, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}

	if err := adapter.Commit(ctx, dir, "initial"); err != nil {
		cmd = exec.Command("git",
			"-c", "user.email=test@test.com",
			"-c", "user.name=Test User",
			"commit", "-m", "initial")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdapter_BasicGitOps(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	repoDir := t.TempDir()

	requireNoErr(t, adapter.Init(ctx, repoDir), "init")

	setupRepo(t, adapter, repoDir)

	_, err := adapter.GetCommitHash(ctx, repoDir)
	requireNoErr(t, err, "get commit hash")

	branches, err := adapter.GetBranches(ctx, repoDir)
	requireNoErr(t, err, "get branches")
	requireTrue(t, len(branches) > 0, "expected branches")

	branch, err := adapter.GetCurrentBranch(ctx, repoDir)
	requireNoErr(t, err, "get current branch")
	requireTrue(t, branch != "", "expected current branch")

	clean, err := adapter.IsClean(ctx, repoDir)
	requireNoErr(t, err, "is clean")
	requireTrue(t, clean, "expected clean repo")

	status, err := adapter.GetStatus(ctx, repoDir)
	requireNoErr(t, err, "get status")
	requireTrue(t, status.Clean, "expected clean status")

	requireNoErr(
		t,
		os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("changed"), 0o600),
		"write file",
	)

	clean, err = adapter.IsClean(ctx, repoDir)
	requireNoErr(t, err, "is clean after change")
	requireTrue(t, !clean, "expected dirty repo")

	status, err = adapter.GetStatus(ctx, repoDir)
	requireNoErr(t, err, "get status after change")
	requireTrue(t, !status.Clean, "expected dirty status")

	logEntries, err := adapter.GetLog(ctx, repoDir, 1)
	requireNoErr(t, err, "get log")
	requireTrue(t, len(logEntries) > 0, "expected log entries")
}

func TestAdapter_RemotesAndConfig(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	repoDir := t.TempDir()
	remoteDir := t.TempDir()

	setupRepo(t, adapter, repoDir)

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	remotes, err := adapter.GetRemotes(ctx, repoDir)
	if err != nil || len(remotes) == 0 {
		t.Fatal("expected remotes")
	}

	if err := adapter.Push(ctx, repoDir, "origin", "HEAD"); err != nil {
		t.Fatal(err)
	}

	if err := adapter.Fetch(ctx, repoDir, "origin"); err != nil {
		t.Fatal(err)
	}

	_, _ = adapter.ConfigGet(ctx, repoDir, "user.name")
}

func TestAdapter_RepoRootAndIgnored(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	repoDir := t.TempDir()
	setupRepo(t, adapter, repoDir)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	root, err := adapter.RepoRoot(ctx)
	if err != nil || root != repoDir {
		t.Fatal("expected repo root")
	}

	ignoreFile := filepath.Join(repoDir, ".gitignore")
	if err := os.WriteFile(ignoreFile, []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "ignored.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	paths, err := adapter.ListIgnoredUntracked(ctx, repoDir)
	if err != nil || len(paths) == 0 {
		t.Fatal("expected ignored/untracked files")
	}
}

func TestAdapter_BranchCheckout(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	repoDir := t.TempDir()
	setupRepo(t, adapter, repoDir)

	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if err := adapter.CheckoutBranch(ctx, repoDir, "feature"); err != nil {
		t.Fatal(err)
	}
}

func requireNoErr(t *testing.T, err error, message string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func requireTrue(t *testing.T, cond bool, message string) {
	t.Helper()
	if !cond {
		t.Fatal(message)
	}
}
