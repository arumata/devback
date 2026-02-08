package main

import (
	"log/slog"
	"os"
	"testing"

	"github.com/arumata/devback/internal/adapters/config"
	"github.com/arumata/devback/internal/adapters/filesystem"
	"github.com/arumata/devback/internal/usecase"
)

func TestRootCmd_ParsesFlags(t *testing.T) {
	cfg := &usecase.Config{}
	run := func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) error {
		if !cfg.Verbose || !cfg.DryRun || cfg.PrintRepoKey {
			t.Fatalf("expected flags to be set: %+v", cfg)
		}
		if logger == nil {
			t.Fatal("expected logger to be set")
		}
		return nil
	}

	testLocks := func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) error {
		if logger == nil {
			t.Fatal("expected logger to be set")
		}
		return nil
	}
	printRepoKey := func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) (string, error) {
		if logger == nil {
			t.Fatal("expected logger to be set")
		}
		return "repo-key", nil
	}
	depsFactory := func(logger *slog.Logger) *usecase.Dependencies {
		return &usecase.Dependencies{
			FileSystem: filesystem.New(logger),
			Config:     config.New(logger),
		}
	}
	cmd, _ := newRootCmd(cfg, depsFactory, run, testLocks, printRepoKey)
	cmd.SetArgs([]string{"--dry-run", "-v", "--print-repo-key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmd_RejectsPositionalArgs(t *testing.T) {
	cfg := &usecase.Config{}
	noop := func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) error { return nil }
	noopKey := func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) (string, error) {
		return "", nil
	}
	depsFactory := func(logger *slog.Logger) *usecase.Dependencies {
		return &usecase.Dependencies{
			FileSystem: filesystem.New(logger),
			Config:     config.New(logger),
		}
	}
	cmd, _ := newRootCmd(cfg, depsFactory, noop, noop, noopKey)
	cmd.SetArgs([]string{"/backups"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for positional argument, got nil")
	}
}

func TestSetupLogger(t *testing.T) {
	if setupLogger(true) == nil {
		t.Fatal("expected logger for verbose")
	}
	if setupLogger(false) == nil {
		t.Fatal("expected logger for non-verbose")
	}
}

func TestShouldUseColor_NoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if shouldUseColor(f) {
		t.Error("shouldUseColor must return false when NO_COLOR is set")
	}
}

func TestShouldUseColor_TermDumb(t *testing.T) {
	t.Setenv("TERM", "dumb")
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if shouldUseColor(f) {
		t.Error("shouldUseColor must return false when TERM=dumb")
	}
}

func TestShouldUseColor_NonTerminalFd(t *testing.T) {
	// Ensure NO_COLOR is unset (use t.Setenv to get automatic restore).
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		t.Setenv("NO_COLOR", "placeholder")
	}
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	// Ensure TERM is not "dumb".
	t.Setenv("TERM", "xterm-256color")

	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if shouldUseColor(f) {
		t.Error("shouldUseColor must return false for non-terminal file descriptor")
	}
}

func TestRunMain_NoArgs(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	homeDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	if code := runMain(); code == 0 {
		t.Fatalf("expected non-zero exit code, got %d", code)
	}
}
