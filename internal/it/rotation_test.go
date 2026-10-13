package it

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arumata/devback/internal/app"
	"github.com/arumata/devback/internal/usecase"
)

func TestBackup_OversizedSnapshotSurvivesRotation(t *testing.T) {
	for _, dedup := range []bool{false, true} {
		name := "full-copy"
		if dedup {
			name = "hardlinks"
		}
		t.Run(name, func(t *testing.T) {
			tempDir := t.TempDir()
			repo := setupHardlinkRepo(t, tempDir)
			restore := chdirForTest(t, repo)
			defer restore()
			payload := bytes.Repeat([]byte("x"), 2*1024*1024)
			if err := os.WriteFile(filepath.Join(repo, "large.bin"), payload, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := &usecase.Config{
				BackupDir:         filepath.Join(tempDir, "backups"),
				MaxTotalGBPerRepo: 1,
				// Reduce the effective limit to 1 MiB without copying a GiB in tests.
				SizeMarginMB: -1023,
				LinkDedup:    dedup,
			}
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			deps := app.NewDefaultDependencies(logger)
			var previous string
			for run := 0; run < 2; run++ {
				logs.Reset()
				if _, err := usecase.Backup(context.Background(), cfg, deps, logger); err != nil {
					t.Fatalf("backup failed: %v", err)
				}
				matches, err := filepath.Glob(filepath.Join(cfg.BackupDir, "*", "*", "*", ".done"))
				if err != nil || len(matches) != 1 {
					t.Fatalf("expected one retained backup: %v (%v); logs: %s", matches, err, &logs)
				}
				snap := filepath.Dir(matches[0])
				if snap == previous {
					t.Fatal("rotation must retain the newly created backup")
				}
				// #nosec G304 -- snapshot path is controlled by the test harness.
				data, err := os.ReadFile(filepath.Join(snap, "large.bin"))
				if err != nil || !bytes.Equal(data, payload) {
					t.Fatalf("retained data differs: %v", err)
				}
				if !strings.Contains(logs.String(), "size limit exceeded") ||
					!strings.Contains(logs.String(), "Backup finished → "+snap) {
					t.Fatalf("expected warning and success referencing the retained backup: %s", &logs)
				}
				previous = snap
			}
		})
	}
}
