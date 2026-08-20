package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Git config variable names cannot contain underscores, so TOML rotation
// keys map to camelCase repo-level keys.
const (
	gitKeyKeepCount    = "backup.keepCount"
	gitKeyKeepDays     = "backup.keepDays"
	gitKeyMaxTotalGb   = "backup.maxTotalGb"
	gitKeySizeMarginMb = "backup.sizeMarginMb"
	gitKeyNoSize       = "backup.noSize"
	gitKeyLinkDedup    = "backup.linkDedup"
)

// ApplyRepoRotationOverrides overlays repo-level git config rotation settings
// onto the runtime config. Resolution order matches backup.enabled:
// worktree config first, then merged git config. Invalid values are a hard
// error: silently falling back to global rotation limits could delete
// snapshots the user meant to keep.
func ApplyRepoRotationOverrides(
	ctx context.Context,
	deps *Dependencies,
	cfg *Config,
	repoRoot string,
	bc *backupContext,
) error {
	if deps == nil || deps.Git == nil || cfg == nil {
		return nil
	}
	intOverrides := []struct {
		key string
		dst *int
	}{
		{gitKeyKeepCount, &cfg.KeepCount},
		{gitKeyKeepDays, &cfg.KeepDays},
		{gitKeyMaxTotalGb, &cfg.MaxTotalGBPerRepo},
		{gitKeySizeMarginMb, &cfg.SizeMarginMB},
	}
	for _, o := range intOverrides {
		raw := readRepoConfig(ctx, deps.Git, repoRoot, true, o.key)
		if raw == "" {
			continue
		}
		value, err := parseRotationInt(raw)
		if err != nil {
			return fmt.Errorf("invalid git config %s=%q: %v: %w", o.key, raw, err, ErrCritical)
		}
		*o.dst = value
		bc.vlogf("→ Repo override: %s = %d", o.key, value)
	}

	boolOverrides := []struct {
		key string
		dst *bool
	}{
		{gitKeyNoSize, &cfg.NoSize},
		{gitKeyLinkDedup, &cfg.LinkDedup},
	}
	for _, o := range boolOverrides {
		raw := readRepoConfig(ctx, deps.Git, repoRoot, true, o.key)
		if raw == "" {
			continue
		}
		value, err := parseRotationBool(raw)
		if err != nil {
			return fmt.Errorf("invalid git config %s=%q: %v: %w", o.key, raw, err, ErrCritical)
		}
		*o.dst = value
		bc.vlogf("→ Repo override: %s = %t", o.key, value)
	}
	return nil
}

// parseRotationInt parses a non-negative integer override value.
func parseRotationInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("expected integer")
	}
	if value < 0 {
		return 0, fmt.Errorf("expected non-negative integer")
	}
	return value, nil
}

// ParseGitBool parses a git-style boolean value ("1"/"true"/"yes"/"on" and
// their negatives); anything else is an error.
func ParseGitBool(raw string) (bool, error) {
	return parseRotationBool(raw)
}

// parseRotationBool parses a git-style boolean override value.
func parseRotationBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean")
	}
}
