//nolint:gci,gofumpt
package it

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/arumata/devback/internal/app"
	"github.com/arumata/devback/internal/usecase"
)

func setupHardlinkRepo(t *testing.T, tempDir string) string {
	t.Helper()
	repoPath := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "it@example.com"},
		{"config", "user.name", "it"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("*.bin\nmutable.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repoPath
}

func runHardlinkBackup(t *testing.T, cfg *usecase.Config, deps *usecase.Dependencies) {
	t.Helper()
	if _, err := usecase.Backup(context.Background(), cfg, deps, slog.Default()); err != nil {
		t.Fatalf("backup failed: %v", err)
	}
}

func snapshotDirs(t *testing.T, repoDir string) []string {
	t.Helper()
	pattern := filepath.Join(repoDir, "*", "*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []string
	for _, m := range matches {
		if _, err := os.Stat(filepath.Join(m, ".done")); err == nil {
			snaps = append(snaps, m)
		}
	}
	return snaps
}

func sameInode(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatalf("stat %s: %v", a, err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatalf("stat %s: %v", b, err)
	}
	return os.SameFile(ai, bi)
}

func TestBackup_HardlinkDedupAcrossSnapshots(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := setupHardlinkRepo(t, tempDir)
	restoreDir := chdirForTest(t, repoPath)
	defer restoreDir()

	heavy := filepath.Join(repoPath, "heavy.bin")
	if err := os.WriteFile(heavy, make([]byte, 256*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	mutable := filepath.Join(repoPath, "mutable.txt")
	if err := os.WriteFile(mutable, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Backdate mtimes so the rewritten mutable file cannot collide with the
	// first snapshot within the same second.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(heavy, past, past); err != nil {
		t.Fatal(err)
	}

	deps := app.NewDefaultDependencies(newDiscardLogger())
	cfg := &usecase.Config{
		BackupDir: filepath.Join(tempDir, "backup"),
		NoSize:    true,
		LinkDedup: true,
	}

	runHardlinkBackup(t, cfg, deps)
	if err := os.WriteFile(mutable, []byte("v2-changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	runHardlinkBackup(t, cfg, deps)

	repoDirs, err := filepath.Glob(filepath.Join(cfg.BackupDir, "*"))
	if err != nil || len(repoDirs) != 1 {
		t.Fatalf("expected single repo dir, got %v (%v)", repoDirs, err)
	}
	snaps := snapshotDirs(t, repoDirs[0])
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %v", snaps)
	}

	if !sameInode(t, filepath.Join(snaps[0], "heavy.bin"), filepath.Join(snaps[1], "heavy.bin")) {
		t.Fatal("unchanged heavy file must share one inode across snapshots")
	}
	if sameInode(t, filepath.Join(snaps[0], "mutable.txt"), filepath.Join(snaps[1], "mutable.txt")) {
		t.Fatal("changed file must have its own inode in the new snapshot")
	}
	if sameInode(t, heavy, filepath.Join(snaps[1], "heavy.bin")) {
		t.Fatal("snapshots must never link to files of the source repository")
	}

	assertRotationKeepsSharedData(t, cfg, deps, repoDirs[0])
}

// assertRotationKeepsSharedData rotates down to one snapshot and checks the
// hard-link-shared data survives in the remaining one.
func assertRotationKeepsSharedData(
	t *testing.T,
	cfg *usecase.Config,
	deps *usecase.Dependencies,
	repoDir string,
) {
	t.Helper()
	cfg.KeepCount = 1
	runHardlinkBackup(t, cfg, deps)
	remaining := snapshotDirs(t, repoDir)
	if len(remaining) != 1 {
		t.Fatalf("expected rotation to keep 1 snapshot, got %v", remaining)
	}
	// #nosec G304 -- test paths are controlled by the test harness.
	data, err := os.ReadFile(filepath.Join(remaining[0], "heavy.bin"))
	if err != nil || len(data) != 256*1024 {
		t.Fatalf("shared file must stay intact after rotation: %d bytes, %v", len(data), err)
	}
}

func TestBackup_HardlinkDedupDisabled(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := setupHardlinkRepo(t, tempDir)
	restoreDir := chdirForTest(t, repoPath)
	defer restoreDir()

	heavy := filepath.Join(repoPath, "heavy.bin")
	if err := os.WriteFile(heavy, make([]byte, 64*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(heavy, past, past); err != nil {
		t.Fatal(err)
	}

	deps := app.NewDefaultDependencies(newDiscardLogger())
	cfg := &usecase.Config{
		BackupDir: filepath.Join(tempDir, "backup"),
		NoSize:    true,
		LinkDedup: false,
	}

	runHardlinkBackup(t, cfg, deps)
	runHardlinkBackup(t, cfg, deps)

	repoDirs, err := filepath.Glob(filepath.Join(cfg.BackupDir, "*"))
	if err != nil || len(repoDirs) != 1 {
		t.Fatalf("expected single repo dir, got %v (%v)", repoDirs, err)
	}
	snaps := snapshotDirs(t, repoDirs[0])
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %v", snaps)
	}
	if sameInode(t, filepath.Join(snaps[0], "heavy.bin"), filepath.Join(snaps[1], "heavy.bin")) {
		t.Fatal("disabled dedup must copy files, not link them")
	}
}
