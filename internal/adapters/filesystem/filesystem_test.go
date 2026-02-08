package filesystem

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestCreateDirExclusive(t *testing.T) {
	ctx := context.Background()
	adapter := New(slog.Default())
	root := t.TempDir()
	path := filepath.Join(root, "exclusive")

	if err := adapter.CreateDirExclusive(ctx, path, 0o755); err != nil {
		t.Fatalf("expected first create to succeed: %v", err)
	}
	if err := adapter.CreateDirExclusive(ctx, path, 0o755); err == nil {
		t.Fatal("expected second create to fail")
	} else if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
}

func TestCreateDirExclusive_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not reliable on windows")
	}
	umask := syscall.Umask(0)
	defer syscall.Umask(umask)

	ctx := context.Background()
	adapter := New(slog.Default())
	root := t.TempDir()
	path := filepath.Join(root, "perm")

	if err := adapter.CreateDirExclusive(ctx, path, 0o700); err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected stat to succeed: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("expected mode 0700, got %o", info.Mode().Perm())
	}
}

func TestCreateDirExclusive_InvalidPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not reliable on windows")
	}
	umask := syscall.Umask(0)
	defer syscall.Umask(umask)

	ctx := context.Background()
	adapter := New(slog.Default())
	root := t.TempDir()
	path := filepath.Join(root, "invalid-perm")

	if err := adapter.CreateDirExclusive(ctx, path, -1); err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected stat to succeed: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected mode 0755, got %o", info.Mode().Perm())
	}
}
