package noop

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

func TestAdapter_NoopFileSystem(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())

	_, err := adapter.ReadFile(ctx, "path")
	expectErr(t, err, "ReadFile")
	expectErr(t, adapter.WriteFile(ctx, "path", []byte("data"), 0o644), "WriteFile")
	expectErr(t, adapter.CreateDir(ctx, "path", 0o755), "CreateDir")
	expectErr(t, adapter.RemoveAll(ctx, "path"), "RemoveAll")
	_, err = adapter.Stat(ctx, "path")
	expectErr(t, err, "Stat")
	_, err = adapter.Lstat(ctx, "path")
	expectErr(t, err, "Lstat")
	expectErr(t, adapter.Walk(ctx, "root", nil), "Walk")
	_, err = adapter.ReadDir(ctx, "root")
	expectErr(t, err, "ReadDir")
	_, err = adapter.Glob(ctx, "*")
	expectErr(t, err, "Glob")
	expectErr(t, adapter.Copy(ctx, "src", "dst"), "Copy")
	expectErr(t, adapter.Move(ctx, "src", "dst"), "Move")
	_, err = adapter.Readlink(ctx, "path")
	expectErr(t, err, "Readlink")
	expectErr(t, adapter.Symlink(ctx, "target", "path"), "Symlink")
	expectErr(t, adapter.Chmod(ctx, "path", 0o600), "Chmod")

	now := time.Now()
	expectErr(t, adapter.Chtimes(ctx, "path", now, now), "Chtimes")
	_, err = adapter.GetWorkingDir(ctx)
	expectErr(t, err, "GetWorkingDir")
	_, err = adapter.Abs(ctx, ".")
	expectErr(t, err, "Abs")
	_, err = adapter.TempDir(ctx, "", "pref")
	expectErr(t, err, "TempDir")

	expectEmptyString(t, adapter.Join("a", "b"), "Join")
	expectEmptyString(t, adapter.Base("path"), "Base")
	expectEmptyString(t, adapter.Dir("path"), "Dir")
	expectEmptyString(t, adapter.Ext("path"), "Ext")
}

func TestAdapter_NoopGit(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())

	expectErr(t, adapter.Init(ctx, "path"), "Init")
	expectErr(t, adapter.Add(ctx, "repo", []string{"file"}), "Add")
	expectErr(t, adapter.Commit(ctx, "repo", "msg"), "Commit")
	_, err := adapter.GetCommitHash(ctx, "repo")
	expectErr(t, err, "GetCommitHash")
	_, err = adapter.GetRemotes(ctx, "repo")
	expectErr(t, err, "GetRemotes")
	expectErr(t, adapter.Fetch(ctx, "repo", "origin"), "Fetch")
	expectErr(t, adapter.Push(ctx, "repo", "origin", "main"), "Push")
	_, err = adapter.GetBranches(ctx, "repo")
	expectErr(t, err, "GetBranches")
	_, err = adapter.GetCurrentBranch(ctx, "repo")
	expectErr(t, err, "GetCurrentBranch")
	expectErr(t, adapter.CheckoutBranch(ctx, "repo", "main"), "CheckoutBranch")
	_, err = adapter.IsClean(ctx, "repo")
	expectErr(t, err, "IsClean")
	_, err = adapter.GetStatus(ctx, "repo")
	expectErr(t, err, "GetStatus")
	_, err = adapter.GetLog(ctx, "repo", 1)
	expectErr(t, err, "GetLog")
	_, err = adapter.RepoRoot(ctx)
	expectErr(t, err, "RepoRoot")
	_, err = adapter.ConfigGet(ctx, "repo", "key")
	expectErr(t, err, "ConfigGet")
	_, err = adapter.ConfigGetWorktree(ctx, "repo", "key")
	expectErr(t, err, "ConfigGetWorktree")
	expectErr(t, adapter.ConfigSetWorktree(ctx, "repo", "key", "value"), "ConfigSetWorktree")
	_, err = adapter.ListIgnoredUntracked(ctx, "repo")
	expectErr(t, err, "ListIgnoredUntracked")
}

func TestAdapter_NoopLock(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())

	expectErr(t, adapter.AcquireLock(ctx, "path", usecase.LockInfo{}), "AcquireLock")
	expectErr(t, adapter.ReleaseLock(ctx, "path"), "ReleaseLock")
	_, _, err := adapter.IsLocked(ctx, "path")
	expectErr(t, err, "IsLocked")
	expectErr(t, adapter.RefreshLock(ctx, "path"), "RefreshLock")
}

func TestAdapter_NoopProcess(t *testing.T) {
	adapter := New(slog.Default())

	expectZeroInt(t, adapter.GetPID(), "GetPID")
}

func TestAdapter_NoopConfigAndTemplates(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())

	_, err := adapter.Load(ctx, "path")
	expectErr(t, err, "Load")
	expectErr(t, adapter.Save(ctx, "path", usecase.ConfigFile{}), "Save")
	_, err = adapter.List(ctx)
	expectErr(t, err, "List")
	_, err = adapter.Read(ctx, "name")
	expectErr(t, err, "Read")
}

func expectErr(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error for %s", name)
	}
}

func expectEmptyString(t *testing.T, value, name string) {
	t.Helper()
	if value != "" {
		t.Fatalf("expected empty %s", name)
	}
}

func expectZeroInt(t *testing.T, value int, name string) {
	t.Helper()
	if value != 0 {
		t.Fatalf("expected zero %s", name)
	}
}
