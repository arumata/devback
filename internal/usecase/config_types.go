package usecase

// ConfigFile describes TOML configuration structure.
type ConfigFile struct {
	Backup        BackupConfig        `toml:"backup"`
	Notifications NotificationsConfig `toml:"notifications"`
	Logging       LoggingConfig       `toml:"logging"`
	RepoKey       RepoKeyConfig       `toml:"repo_key"`
}

// BackupConfig holds backup-related settings.
type BackupConfig struct {
	BaseDir      string `toml:"base_dir"`
	KeepCount    int    `toml:"keep_count"`
	KeepDays     int    `toml:"keep_days"`
	MaxTotalGB   int    `toml:"max_total_gb"`
	SizeMarginMB int    `toml:"size_margin_mb"`
	NoSize       bool   `toml:"no_size"`
}

// NotificationsConfig holds notification settings.
type NotificationsConfig struct {
	Enabled bool   `toml:"enabled"`
	Sound   string `toml:"sound"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Dir   string `toml:"dir"`
	Level string `toml:"level"`
}

// RepoKeyConfig holds repository key generation settings.
type RepoKeyConfig struct {
	Style           string `toml:"style"`
	AutoRemoteMerge bool   `toml:"auto_remote_merge"`
	RemoteHashLen   int    `toml:"remote_hash_len"`
}

const defaultTemplatesDir = "~/.local/share/devback/templates/hooks"

const defaultRepoTemplatesDir = "~/.local/share/devback/repo-templates"

// SuggestedBackupDir is the recommended default for backup.base_dir.
const SuggestedBackupDir = "~/.local/share/devback/backups"

// DefaultTemplatesDir returns default templates directory for git hooks.
func DefaultTemplatesDir() string {
	return defaultTemplatesDir
}

// DefaultRepoTemplatesDir returns default directory for repository templates.
func DefaultRepoTemplatesDir() string {
	return defaultRepoTemplatesDir
}

// DefaultConfigFile returns default TOML configuration.
func DefaultConfigFile() ConfigFile {
	return ConfigFile{
		Backup: BackupConfig{
			BaseDir:      "",
			KeepCount:    30,
			KeepDays:     90,
			MaxTotalGB:   10,
			SizeMarginMB: 0,
			NoSize:       true,
		},
		Notifications: NotificationsConfig{
			Enabled: true,
			Sound:   "default",
		},
		Logging: LoggingConfig{
			Dir:   "~/.local/state/devback/logs",
			Level: "info",
		},
		RepoKey: RepoKeyConfig{
			Style:           repoKeyStyleAuto,
			AutoRemoteMerge: false,
			RemoteHashLen:   8,
		},
	}
}
