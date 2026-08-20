package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func overridesGit(local, worktree map[string]string) *mockGit {
	return &mockGit{
		ConfigGetFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if v, ok := local[key]; ok {
				return v, nil
			}
			return "", fmt.Errorf("not found")
		},
		ConfigGetWorktreeFunc: func(ctx context.Context, repoPath, key string) (string, error) {
			if v, ok := worktree[key]; ok {
				return v, nil
			}
			return "", fmt.Errorf("not found")
		},
	}
}

func applyOverridesForTest(t *testing.T, cfg *Config, local, worktree map[string]string) error {
	t.Helper()
	deps := &Dependencies{Git: overridesGit(local, worktree), FileSystem: newTestFileSystem()}
	return ApplyRepoRotationOverrides(context.Background(), deps, cfg, "/repo", newTestBackupContext(false))
}

func TestApplyRepoRotationOverrides_AppliesValues(t *testing.T) {
	cfg := &Config{KeepCount: 30, KeepDays: 90, MaxTotalGBPerRepo: 10, SizeMarginMB: 0, NoSize: true}
	local := map[string]string{
		"backup.keepCount":    "8",
		"backup.keepDays":     "14",
		"backup.maxTotalGb":   "2",
		"backup.sizeMarginMb": "100",
		"backup.noSize":       "false",
	}
	if err := applyOverridesForTest(t, cfg, local, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KeepCount != 8 || cfg.KeepDays != 14 || cfg.MaxTotalGBPerRepo != 2 || cfg.SizeMarginMB != 100 {
		t.Fatalf("int overrides not applied: %+v", cfg)
	}
	if cfg.NoSize {
		t.Fatal("bool override not applied")
	}
}

func TestApplyRepoRotationOverrides_KeepsGlobalsWhenUnset(t *testing.T) {
	cfg := &Config{KeepCount: 30, KeepDays: 90, MaxTotalGBPerRepo: 10, NoSize: true}
	if err := applyOverridesForTest(t, cfg, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KeepCount != 30 || cfg.KeepDays != 90 || cfg.MaxTotalGBPerRepo != 10 || !cfg.NoSize {
		t.Fatalf("globals must stay untouched: %+v", cfg)
	}
}

func TestApplyRepoRotationOverrides_WorktreeWinsOverLocal(t *testing.T) {
	cfg := &Config{KeepCount: 30}
	local := map[string]string{"backup.keepCount": "9"}
	worktree := map[string]string{"backup.keepCount": "5"}
	if err := applyOverridesForTest(t, cfg, local, worktree); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KeepCount != 5 {
		t.Fatalf("worktree override must win, got %d", cfg.KeepCount)
	}
}

func TestApplyRepoRotationOverrides_InvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"non-integer":  {"backup.keepCount": "abc"},
		"negative":     {"backup.keepDays": "-1"},
		"invalid bool": {"backup.noSize": "maybe"},
	}
	for name, local := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{KeepCount: 30, KeepDays: 90}
			err := applyOverridesForTest(t, cfg, local, nil)
			if err == nil {
				t.Fatal("expected error for invalid override")
			}
			if !errors.Is(err, ErrCritical) {
				t.Fatalf("expected ErrCritical, got %v", err)
			}
			if cfg.KeepCount != 30 || cfg.KeepDays != 90 {
				t.Fatalf("config must stay untouched on error: %+v", cfg)
			}
		})
	}
}

func TestParseRotationBool(t *testing.T) {
	truthy := []string{"1", "true", "YES", "on"}
	falsy := []string{"0", "false", "No", "off"}
	for _, v := range truthy {
		got, err := parseRotationBool(v)
		if err != nil || !got {
			t.Fatalf("expected %q to parse as true, got %v %v", v, got, err)
		}
	}
	for _, v := range falsy {
		got, err := parseRotationBool(v)
		if err != nil || got {
			t.Fatalf("expected %q to parse as false, got %v %v", v, got, err)
		}
	}
	if _, err := parseRotationBool("2"); err == nil {
		t.Fatal("expected error for invalid bool")
	}
}
