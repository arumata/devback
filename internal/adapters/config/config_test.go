package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/arumata/devback/internal/usecase"
)

func TestAdapter_LoadMissingReturnsDefaults(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg, err := adapter.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(cfg, usecase.DefaultConfigFile()) {
		t.Fatal("expected default config to be returned")
	}
}

func TestAdapter_SaveAndLoad(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())
	path := filepath.Join(t.TempDir(), "config.toml")

	original := usecase.ConfigFile{
		Backup: usecase.BackupConfig{
			BaseDir:      "/backup",
			KeepCount:    15,
			KeepDays:     60,
			MaxTotalGB:   5,
			SizeMarginMB: 12,
			NoSize:       false,
		},
		Notifications: usecase.NotificationsConfig{
			Enabled: false,
			Sound:   "Glass",
		},
		Logging: usecase.LoggingConfig{
			Dir:   "/logs",
			Level: "debug",
		},
		RepoKey: usecase.RepoKeyConfig{
			Style:           "remote-hierarchy",
			AutoRemoteMerge: true,
			RemoteHashLen:   12,
		},
	}

	if err := adapter.Save(context.Background(), path, original); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	loaded, err := adapter.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if !reflect.DeepEqual(loaded, original) {
		t.Fatal("loaded config does not match saved config")
	}
}

func TestAdapter_SaveProducesCommentedTOML(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())
	path := filepath.Join(t.TempDir(), "config.toml")

	if err := adapter.Save(context.Background(), path, usecase.DefaultConfigFile()); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	data, err := os.ReadFile(path) // #nosec G304 - test data
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	content := string(data)

	for _, marker := range []string{
		"# DevBack Configuration",
		"# ── Backup Settings",
		"# ── Desktop Notifications",
		"# ── Logging",
		"# ── Repository Key",
		"[backup]",
		"[notifications]",
		"[logging]",
		"[repo_key]",
	} {
		if !strings.Contains(content, marker) {
			t.Errorf("expected config to contain %q", marker)
		}
	}
}

func TestAdapter_LoadInvalidTOML(t *testing.T) {
	t.Parallel()
	adapter := New(slog.Default())
	path := filepath.Join(t.TempDir(), "config.toml")

	// #nosec G306 - test data does not require restrictive permissions.
	if err := os.WriteFile(path, []byte("backup = ["), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	if _, err := adapter.Load(context.Background(), path); err == nil {
		t.Fatal("expected error for invalid toml")
	}
}
