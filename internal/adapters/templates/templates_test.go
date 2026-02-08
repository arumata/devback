package templates

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestAdapter_List(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	entries, err := adapter.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected templates list to be non-empty")
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(entries))
	}

	modes := make(map[string]int, len(entries))
	for _, entry := range entries {
		modes[entry.Name] = entry.Mode
	}

	if modes["post-commit"] != templateExecPerm {
		t.Fatal("expected post-commit to be executable")
	}
	if modes["post-merge"] != templateExecPerm {
		t.Fatal("expected post-merge to be executable")
	}
	if modes["post-rewrite"] != templateExecPerm {
		t.Fatal("expected post-rewrite to be executable")
	}
}

func TestAdapter_Read(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	data, err := adapter.Read(context.Background(), "post-commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := repoRoot(t)
	assetPath := filepath.Join(root, "assets", "git-templates", "post-commit")

	// #nosec G304 -- test paths are controlled by the test harness.
	disk, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset file: %v", err)
	}

	if !bytes.Equal(data, disk) {
		t.Fatal("embedded template does not match asset file")
	}
}

func TestAdapter_ReadInvalidName(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	if _, err := adapter.Read(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty template name")
	}
}

func TestAdapter_ListRepo(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	entries, err := adapter.ListRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected repo templates list to be non-empty")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 repo template, got %d", len(entries))
	}
	if entries[0].Name != "devbackignore" {
		t.Fatalf("expected devbackignore, got %s", entries[0].Name)
	}
	if entries[0].Mode != templateFilePerm {
		t.Fatalf("expected mode %o, got %o", templateFilePerm, entries[0].Mode)
	}
}

func TestAdapter_ReadRepo(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	data, err := adapter.ReadRepo(context.Background(), "devbackignore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := repoRoot(t)
	assetPath := filepath.Join(root, "assets", "repo-templates", "devbackignore")

	// #nosec G304 -- test paths are controlled by the test harness.
	disk, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset file: %v", err)
	}

	if !bytes.Equal(data, disk) {
		t.Fatal("embedded repo template does not match asset file")
	}
}

func TestAdapter_ReadRepoInvalidName(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())

	if _, err := adapter.ReadRepo(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty repo template name")
	}
}
