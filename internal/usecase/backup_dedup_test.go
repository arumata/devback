package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFileWithMtime(t *testing.T, path, content string, mode os.FileMode, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func mustSameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}

func copyFileForTest(t *testing.T, src, dst, prevPath string) bool {
	t.Helper()
	ctx := context.Background()
	deps := &Dependencies{FileSystem: newTestFileSystem()}
	// Callers of copyFile create the destination directory beforehand.
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		t.Fatal(err)
	}
	info, err := deps.FileSystem.Lstat(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := copyFile(ctx, deps, src, dst, info, prevPath)
	if err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	return linked
}

func TestCopyFile_LinksUnchangedFromPrev(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	src := filepath.Join(dir, "src", "file.bin")
	prev := filepath.Join(dir, "prev", "file.bin")
	dst := filepath.Join(dir, "dst", "file.bin")
	writeFileWithMtime(t, src, "payload", 0o640, mtime)
	writeFileWithMtime(t, prev, "payload", 0o640, mtime)

	if !copyFileForTest(t, src, dst, prev) {
		t.Fatal("expected hardlink, got copy")
	}
	if !mustSameFile(t, dst, prev) {
		t.Fatal("expected dst to share inode with prev")
	}
	if mustSameFile(t, dst, src) {
		t.Fatal("dst must never share inode with the source repo")
	}
}

func TestCopyFile_CopiesWhenPrevDiffers(t *testing.T) {
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	cases := map[string]func(t *testing.T, prev string){
		"size differs": func(t *testing.T, prev string) {
			writeFileWithMtime(t, prev, "payload-longer", 0o640, mtime)
		},
		"mtime differs": func(t *testing.T, prev string) {
			writeFileWithMtime(t, prev, "payload", 0o640, mtime.Add(-2*time.Second))
		},
		"mode differs": func(t *testing.T, prev string) {
			writeFileWithMtime(t, prev, "payload", 0o600, mtime)
		},
		"prev missing": func(t *testing.T, prev string) {},
		"prev is symlink": func(t *testing.T, prev string) {
			target := prev + ".target"
			writeFileWithMtime(t, target, "payload", 0o640, mtime)
			if err := os.MkdirAll(filepath.Dir(prev), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, prev); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src", "file.bin")
			prev := filepath.Join(dir, "prev", "file.bin")
			dst := filepath.Join(dir, "dst", "file.bin")
			writeFileWithMtime(t, src, "payload", 0o640, mtime)
			setup(t, prev)

			if copyFileForTest(t, src, dst, prev) {
				t.Fatal("expected copy, got hardlink")
			}
			// #nosec G304 -- test paths are controlled by the test harness.
			if data, err := os.ReadFile(dst); err != nil || string(data) != "payload" {
				t.Fatalf("dst content mismatch: %q %v", data, err)
			}
		})
	}
}

func TestCopyFile_NoPrevPathCopies(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	src := filepath.Join(dir, "src", "file.bin")
	dst := filepath.Join(dir, "dst", "file.bin")
	writeFileWithMtime(t, src, "payload", 0o640, mtime)

	if copyFileForTest(t, src, dst, "") {
		t.Fatal("expected copy without prev path")
	}
}

func TestCopyFile_PreservesSourceMtime(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	src := filepath.Join(dir, "src", "file.bin")
	dst := filepath.Join(dir, "dst", "file.bin")
	writeFileWithMtime(t, src, "payload", 0o640, mtime)

	copyFileForTest(t, src, dst, "")

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Unix() != mtime.Unix() {
		t.Fatalf("expected dst mtime %v, got %v", mtime, info.ModTime())
	}
}

func TestCopyFile_LinkErrorFallsBackToCopy(t *testing.T) {
	ctx := context.Background()
	mtime := time.Now().Truncate(time.Second)
	info := &mockFileInfo{name: "file.bin", size: 7, mode: 0o640, modTime: mtime}
	copied := false
	mockFS := &mockFileSystem{
		LstatFunc: func(ctx context.Context, path string) (FileInfo, error) {
			return &mockFileInfo{name: "file.bin", size: 7, mode: 0o640, modTime: mtime}, nil
		},
		LinkFunc: func(ctx context.Context, oldname, newname string) error {
			return fmt.Errorf("link not supported")
		},
		CopyFunc: func(ctx context.Context, src, dst string) error {
			copied = true
			return nil
		},
	}

	linked, err := copyFile(ctx, &Dependencies{FileSystem: mockFS}, "/src/f", "/dst/f", info, "/prev/f")
	if err != nil {
		t.Fatalf("fallback copy must succeed: %v", err)
	}
	if linked {
		t.Fatal("failed link must not be reported as linked")
	}
	if !copied {
		t.Fatal("expected fallback to copy")
	}
}

func TestCopyDirRecursive_LinkDedup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	src := filepath.Join(dir, "src")
	prev := filepath.Join(dir, "prev")
	dst := filepath.Join(dir, "dst")
	writeFileWithMtime(t, filepath.Join(src, "same.bin"), "unchanged", 0o640, mtime)
	writeFileWithMtime(t, filepath.Join(prev, "same.bin"), "unchanged", 0o640, mtime)
	writeFileWithMtime(t, filepath.Join(src, "changed.bin"), "new-data", 0o640, mtime)
	writeFileWithMtime(t, filepath.Join(prev, "changed.bin"), "old", 0o640, mtime)

	result := &BackupResult{}
	if err := copyDirRecursive(
		ctx,
		&Dependencies{FileSystem: newTestFileSystem()},
		src,
		dst,
		prev,
		result,
		newTestBackupContext(false),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LinkedFiles != 1 || result.CopiedFiles != 1 {
		t.Fatalf("expected 1 linked + 1 copied, got %+v", result)
	}
	if !mustSameFile(t, filepath.Join(dst, "same.bin"), filepath.Join(prev, "same.bin")) {
		t.Fatal("unchanged file must share inode with prev snapshot")
	}
	if mustSameFile(t, filepath.Join(dst, "changed.bin"), filepath.Join(prev, "changed.bin")) {
		t.Fatal("changed file must be a fresh copy")
	}
}

func TestCopySelectedFiles_LinkDedup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	src := filepath.Join(dir, "src")
	prev := filepath.Join(dir, "prev")
	dst := filepath.Join(dir, "dst")
	writeFileWithMtime(t, filepath.Join(src, "sub", "same.bin"), "unchanged", 0o640, mtime)
	writeFileWithMtime(t, filepath.Join(prev, "sub", "same.bin"), "unchanged", 0o640, mtime)
	writeFileWithMtime(t, filepath.Join(src, "new.bin"), "fresh", 0o640, mtime)

	result := &BackupResult{}
	if err := copySelectedFiles(
		ctx,
		&Dependencies{FileSystem: newTestFileSystem()},
		[]string{filepath.Join("sub", "same.bin"), "new.bin"},
		src,
		dst,
		prev,
		result,
		newTestBackupContext(false),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LinkedFiles != 1 || result.CopiedFiles != 1 {
		t.Fatalf("expected 1 linked + 1 copied, got %+v", result)
	}
	if !mustSameFile(t, filepath.Join(dst, "sub", "same.bin"), filepath.Join(prev, "sub", "same.bin")) {
		t.Fatal("unchanged file must share inode with prev snapshot")
	}
}

func makeDoneSnapshot(t *testing.T, repoDir, date, timeDir string) string {
	t.Helper()
	snap := filepath.Join(repoDir, date, timeDir)
	if err := os.MkdirAll(snap, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, ".done"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestFindPrevSnapshot(t *testing.T) {
	ctx := context.Background()
	deps := &Dependencies{FileSystem: newTestFileSystem()}
	repoDir := t.TempDir()
	bc := newTestBackupContext(false)

	if got := findPrevSnapshot(ctx, deps, repoDir, &Config{LinkDedup: true}, bc); got != "" {
		t.Fatalf("expected no prev snapshot in empty dir, got %q", got)
	}

	makeDoneSnapshot(t, repoDir, "2024-01-01", "000000")
	newest := makeDoneSnapshot(t, repoDir, "2024-01-02", "120000")

	if got := findPrevSnapshot(ctx, deps, repoDir, &Config{LinkDedup: true}, bc); got != newest {
		t.Fatalf("expected newest snapshot %q, got %q", newest, got)
	}
	if got := findPrevSnapshot(ctx, deps, repoDir, &Config{LinkDedup: false}, bc); got != "" {
		t.Fatalf("disabled dedup must not pick prev snapshot, got %q", got)
	}
}

func TestChargedSnapshotSizesKB_SharedInodes(t *testing.T) {
	ctx := context.Background()
	deps := &Dependencies{FileSystem: newTestFileSystem()}
	repoDir := t.TempDir()
	bc := newTestBackupContext(false)

	old := makeDoneSnapshot(t, repoDir, "2024-01-01", "000000")
	newer := makeDoneSnapshot(t, repoDir, "2024-01-02", "000000")

	shared := filepath.Join(old, "shared.bin")
	if err := os.WriteFile(shared, make([]byte, 200*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(shared, filepath.Join(newer, "shared.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "old-only.bin"), make([]byte, 100*1024), 0o600); err != nil {
		t.Fatal(err)
	}

	snaps, err := listSnapshots(ctx, deps, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	sizes, totalKB := chargedSnapshotSizesKB(ctx, deps, snaps, nil, bc)

	if sizes[1] < 200 || sizes[1] > 201 {
		t.Fatalf("newest snapshot must be charged for the shared inode, got %d KB", sizes[1])
	}
	if sizes[0] < 100 || sizes[0] > 101 {
		t.Fatalf("oldest snapshot must be charged only for its own file, got %d KB", sizes[0])
	}
	if totalKB != sizes[0]+sizes[1] {
		t.Fatalf("total must equal sum of charged sizes: %d != %d+%d", totalKB, sizes[0], sizes[1])
	}
}

func TestApplySizeLimit_SharedInodesRemovesOnlyOldest(t *testing.T) {
	ctx := context.Background()
	deps := &Dependencies{FileSystem: newTestFileSystem()}
	repoDir := t.TempDir()
	bc := newTestBackupContext(false)

	snapPaths := []string{
		makeDoneSnapshot(t, repoDir, "2024-01-01", "000000"),
		makeDoneSnapshot(t, repoDir, "2024-01-02", "000000"),
		makeDoneSnapshot(t, repoDir, "2024-01-03", "000000"),
	}
	shared := filepath.Join(snapPaths[0], "shared.bin")
	if err := os.WriteFile(shared, make([]byte, 2*1024*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, snap := range snapPaths {
		if i > 0 {
			if err := os.Link(shared, filepath.Join(snap, "shared.bin")); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(snap, "unique.bin"), make([]byte, 1024*1024), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Real usage: 2 MiB shared + 3x1 MiB unique = 5 MiB. The naive per-snapshot
	// sum (9 MiB) would remove two snapshots; charged accounting removes one.
	// kbLimit = 1 GiB + margin: margin of -1020 MiB yields a 4 MiB limit.
	cfg := &Config{MaxTotalGBPerRepo: 1, SizeMarginMB: -1020}
	snaps, err := listSnapshots(ctx, deps, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	alive := []bool{true, true, true}
	applySizeLimit(ctx, deps, cfg, false, bc, snaps, alive)

	if alive[0] {
		t.Fatal("oldest snapshot must be removed")
	}
	if !alive[1] || !alive[2] {
		t.Fatalf("newer snapshots must survive: %v", alive)
	}
	if _, err := os.Stat(snapPaths[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest snapshot dir must be deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapPaths[2], "shared.bin")); err != nil {
		t.Fatalf("shared file must survive in newer snapshots: %v", err)
	}
}
