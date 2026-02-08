package usecase

import (
	"fmt"
	"strings"
)

// RuntimeConfigFromFile converts TOML config into runtime config for backup execution.
func RuntimeConfigFromFile(cfg ConfigFile, homeDir string) (*Config, error) {
	cleanHome := strings.TrimSpace(homeDir)
	if cleanHome == "" {
		return nil, fmt.Errorf("home directory is empty: %w", ErrCritical)
	}

	baseDir := strings.TrimSpace(cfg.Backup.BaseDir)
	if baseDir != "" {
		baseDir = expandHomeDir(baseDir, cleanHome)
	}

	style := strings.TrimSpace(cfg.RepoKey.Style)
	if style == "" {
		style = repoKeyStyleAuto
	}

	return &Config{
		BackupDir:         baseDir,
		KeepCount:         cfg.Backup.KeepCount,
		KeepDays:          cfg.Backup.KeepDays,
		MaxTotalGBPerRepo: cfg.Backup.MaxTotalGB,
		SizeMarginMB:      cfg.Backup.SizeMarginMB,
		RepoKeyStyle:      style,
		AutoRemoteMerge:   cfg.RepoKey.AutoRemoteMerge,
		RemoteHashLen:     cfg.RepoKey.RemoteHashLen,
		NoSize:            cfg.Backup.NoSize,
	}, nil
}
