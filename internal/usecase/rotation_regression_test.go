package usecase

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotationPreservesLatestSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int
		dryRun bool
	}{
		{name: "single oversized snapshot", count: 1},
		{name: "multiple oversized snapshots", count: 3},
		{name: "dry run", count: 3, dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := t.TempDir()
			var paths []string
			for i := 0; i < tc.count; i++ {
				snap := makeDoneSnapshot(t, repoDir, "2024-01-01", time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC).Format("150405"))
				file := filepath.Join(snap, "large.bin")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(file, 1024*1024*1024+1); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, snap)
			}
			var logs bytes.Buffer
			bc := newBackupContext(slog.New(slog.NewTextHandler(&logs, nil)), false)
			rotateRepo(context.Background(), &Dependencies{FileSystem: newTestFileSystem()}, repoDir,
				&Config{MaxTotalGBPerRepo: 1}, tc.dryRun, bc, "")
			remaining, err := listSnapshots(context.Background(), &Dependencies{FileSystem: newTestFileSystem()}, repoDir)
			want := 1
			if tc.dryRun {
				want = tc.count
			}
			if err != nil || len(remaining) != want {
				t.Fatalf("expected %d remaining snapshots, got %v (%v)", want, remaining, err)
			}
			if _, err := os.Stat(filepath.Join(paths[len(paths)-1], ".done")); err != nil {
				t.Fatalf("latest completed snapshot must survive: %v", err)
			}
			if !strings.Contains(logs.String(), "size limit exceeded") || !strings.Contains(logs.String(), "retained") {
				t.Fatalf("expected explicit limit warning and retained snapshot: %s", &logs)
			}
		})
	}
}

func TestRotationProtectsCurrentSnapshotAfterClockRollback(t *testing.T) {
	for _, cfg := range []*Config{
		{KeepCount: 1},
		{KeepDays: 1},
		{MaxTotalGBPerRepo: 1, SizeMarginMB: -1024},
	} {
		repoDir := t.TempDir()
		current := makeDoneSnapshot(t, repoDir, "2024-01-01", "000000")
		old := makeDoneSnapshot(t, repoDir, "2024-01-02", "000000")
		past := time.Now().Add(-48 * time.Hour)
		for _, snap := range []string{current, old} {
			if err := os.WriteFile(filepath.Join(snap, "data"), []byte("data"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(filepath.Join(snap, ".done"), past, past); err != nil {
				t.Fatal(err)
			}
		}
		rotateRepo(context.Background(), &Dependencies{FileSystem: newTestFileSystem()}, repoDir,
			cfg, false, newTestBackupContext(false), current)
		if _, err := os.Stat(filepath.Join(current, ".done")); err != nil {
			t.Fatalf("current snapshot must survive with config %+v: %v", cfg, err)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatalf("previous snapshot must be removed with config %+v: %v", cfg, err)
		}
	}
}

type missingDoneAfterRotationFS struct {
	*testFileSystem
	doneStats int
}

func (f *missingDoneAfterRotationFS) Stat(ctx context.Context, path string) (FileInfo, error) {
	if strings.HasSuffix(path, "/.done") {
		f.doneStats++
		if f.doneStats > 1 {
			return nil, os.ErrNotExist
		}
	}
	return f.testFileSystem.Stat(ctx, path)
}

func TestBackupDoesNotReportMissingSnapshotAsSuccess(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "init", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	var logs bytes.Buffer
	bc := newBackupContext(slog.New(slog.NewTextHandler(&logs, nil)), false)
	deps := &Dependencies{
		FileSystem: &missingDoneAfterRotationFS{testFileSystem: newTestFileSystem()},
		Git:        newTestGitAdapter(),
	}
	_, err := handleBackupFlow(context.Background(), &Config{NoSize: true}, deps, repo, t.TempDir(), bc)
	if !errors.Is(err, ErrCritical) || strings.Contains(logs.String(), "Backup finished") {
		t.Fatalf("missing snapshot must fail without success message: %v; %s", err, &logs)
	}
}

type failedRotationRemoveFS struct {
	*testFileSystem
	failedPath string
}

func (f *failedRotationRemoveFS) RemoveAll(ctx context.Context, path string) error {
	if path == f.failedPath {
		return os.ErrPermission
	}
	return f.testFileSystem.RemoveAll(ctx, path)
}

func TestRotationReportsSizeAfterFailedDeletion(t *testing.T) {
	repoDir := t.TempDir()
	old := makeDoneSnapshot(t, repoDir, "2024-01-01", "000000")
	middle := makeDoneSnapshot(t, repoDir, "2024-01-02", "000000")
	current := makeDoneSnapshot(t, repoDir, "2024-01-03", "000000")
	shared := filepath.Join(old, "shared.bin")
	if err := os.WriteFile(shared, make([]byte, 2*1024*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(shared, filepath.Join(middle, "shared.bin")); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	bc := newBackupContext(slog.New(slog.NewTextHandler(&logs, nil)), false)
	deps := &Dependencies{FileSystem: &failedRotationRemoveFS{
		testFileSystem: newTestFileSystem(), failedPath: old,
	}}
	rotateRepo(context.Background(), deps, repoDir,
		&Config{MaxTotalGBPerRepo: 1, SizeMarginMB: -1023}, false, bc, current)
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("failed deletion must leave shared data accounted for: %v", err)
	}
	if _, err := os.Stat(middle); !os.IsNotExist(err) {
		t.Fatalf("middle snapshot must be removed: %v", err)
	}
	if !strings.Contains(logs.String(), "total 2.00 MiB > 1.00 MiB") ||
		!strings.Contains(logs.String(), "size limit exceeded") || !strings.Contains(logs.String(), "permission denied") {
		t.Fatalf("expected failed deletion and actual remaining size warnings: %s", &logs)
	}
}
