package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/arumata/devback/internal/usecase"
)

// Adapter implements ConfigPort using TOML files on disk.
type Adapter struct {
	logger *slog.Logger
}

// New creates a new config adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("config adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// Load reads config from path or returns defaults when file is missing.
func (a *Adapter) Load(ctx context.Context, path string) (usecase.ConfigFile, error) {
	_ = ctx
	if strings.TrimSpace(path) == "" {
		return usecase.ConfigFile{}, errors.New("config path is empty")
	}

	data, err := os.ReadFile(path) // #nosec G304 - path is controlled by usecase
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return usecase.DefaultConfigFile(), nil
		}
		return usecase.ConfigFile{}, err
	}

	cfg := usecase.DefaultConfigFile()
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return usecase.ConfigFile{}, fmt.Errorf("parse config toml: %w", err)
	}

	return cfg, nil
}

// Save writes config to path in TOML format with inline documentation.
func (a *Adapter) Save(ctx context.Context, path string, cfg usecase.ConfigFile) error {
	_ = ctx
	if strings.TrimSpace(path) == "" {
		return errors.New("config path is empty")
	}

	content := renderCommentedTOML(cfg)

	// #nosec G306 G304 - config is not secret, path is controlled by usecase.
	return os.WriteFile(path, []byte(content), 0o644)
}

//nolint:lll // template readability is more important than line length.
func renderCommentedTOML(cfg usecase.ConfigFile) string {
	return fmt.Sprintf(`# DevBack Configuration
# https://github.com/arumata/devback#configuration

# ── Backup Settings ──────────────────────────────────────────────
[backup]

# Base directory for snapshots (required).
# Supports ~, $HOME, ${HOME}. Created automatically.
# Set via: devback init --backup-dir <path>
base_dir = %[1]q

# Maximum number of snapshots to keep per repository.
keep_count = %[2]d

# Maximum snapshot age in days.
keep_days = %[3]d

# Maximum total size (GB) of snapshots per repository.
# Ignored when no_size = true.
max_total_gb = %[4]d

# Margin in MB added to max_total_gb before triggering rotation.
size_margin_mb = %[5]d

# Disable size-based rotation.
# When true, max_total_gb and size_margin_mb are ignored.
no_size = %[6]t

# ── Desktop Notifications ────────────────────────────────────────
[notifications]

# Enable notifications after backup completion.
enabled = %[7]t

# Notification sound ("default" = system default).
sound = %[8]q

# ── Logging ──────────────────────────────────────────────────────
[logging]

# Log directory. Supports ~, $HOME, ${HOME}. Created automatically.
dir = %[9]q

# Minimum log level: debug, info, warn, error.
level = %[10]q

# ── Repository Key ───────────────────────────────────────────────
[repo_key]

# Naming style for snapshot directories:
#   auto             - auto-detect: slug, remote, or name+hash (default)
#   custom           - uses backup.slug (set via: devback setup --slug)
#   remote-hierarchy - host/owner/repo from remote.origin.url
style = %[11]q

# Merge snapshots from clones sharing the same remote.origin.url.
auto_remote_merge = %[12]t

# Hash suffix length for remote-hierarchy style.
remote_hash_len = %[13]d
`,
		cfg.Backup.BaseDir,
		cfg.Backup.KeepCount,
		cfg.Backup.KeepDays,
		cfg.Backup.MaxTotalGB,
		cfg.Backup.SizeMarginMB,
		cfg.Backup.NoSize,
		cfg.Notifications.Enabled,
		cfg.Notifications.Sound,
		cfg.Logging.Dir,
		cfg.Logging.Level,
		cfg.RepoKey.Style,
		cfg.RepoKey.AutoRemoteMerge,
		cfg.RepoKey.RemoteHashLen,
	)
}
