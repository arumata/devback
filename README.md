# DevBack

[![Go Version](https://img.shields.io/github/go-mod/go-version/arumata/devback)](https://go.dev/)
[![License](https://img.shields.io/github/license/arumata/devback)](LICENSE)
[![Release](https://img.shields.io/github/v/release/arumata/devback)](https://github.com/arumata/devback/releases/latest)
[![Homebrew Cask](https://img.shields.io/badge/homebrew-cask-orange)](https://github.com/arumata/homebrew-tap)

A compact CLI tool for creating full backups of Git repositories, including the `.git` directory and all ignored/untracked files.

## Overview

DevBack creates structured repository snapshots with automatic rotation, flexible naming styles, and legacy directory migration. Designed for sysadmins and developers who need reliable Git repository backups.

## Features

- **Full backup**: Includes `.git` directory and all ignored/untracked files
- **Structured snapshots**: Automatic organization by date and time
- **Automatic rotation**: Manage backup size and count
- **Flexible naming**: Multiple directory naming styles
- **Auto-migration**: Automatic migration from legacy directory structures
- **Parallel copy**: Efficient handling of large repositories
- **Global init and hook installation**: `devback init` and `devback setup` commands
- **Status and diagnostics**: `devback status` command
- **Git worktree support**: Correct handling of shared hooks
- **TOML configuration**: Single config file for all commands
- **Standardized exit codes**: For automation and monitoring

## Snapshot Structure

```
<backup_dir>/<repo_key>/<YYYY-MM-DD>/<HHMMSS-NNNNNNNNN>/
├── .partial (created on start)
├── .done    (created after successful completion)
└── .git/    (full copy of the Git repository)
    └── ... (all ignored/untracked files)
```

### Snapshot Time Format

The time directory uses `HHMMSS-NNNNNNNNN` format, where the suffix is nanoseconds to guarantee uniqueness across repeated runs within the same second.

### Atomic Snapshot Reservation

Snapshot directories are reserved atomically via exclusive directory creation and a `.reserve` marker file. This prevents collisions during parallel runs. The marker is removed after a successful backup.

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap arumata/tap
brew install --cask devback
```

### Go Install

```bash
go install github.com/arumata/devback/cmd/app@latest
```

Requires Go 1.24+. The binary will be installed to `$GOPATH/bin` (or `$HOME/go/bin` by default).

### Download Binary

Pre-built binaries for Linux and macOS (amd64/arm64) are available on the
[Releases](https://github.com/arumata/devback/releases) page.

### Build from Source

Requirements: Go 1.24+, Git.

```bash
git clone https://github.com/arumata/devback.git
cd devback

# Build
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE" \
  -o devback ./cmd/app

# Install
sudo install devback /usr/local/bin/
# or
install devback ~/.local/bin/
```

## Usage

### Quick Start

```bash
devback init
cd ~/projects/my-app
devback setup --slug "company/my-app"
devback status
```

After `setup`, hooks automatically trigger a backup after `git commit`, `git merge`,
and `git rebase --rebase-merges`/`git commit --amend` (via `post-rewrite`).
To run hook commands manually:

```bash
devback hook post-commit
devback hook post-merge
devback hook post-rewrite rebase
```

To run a backup manually:

```bash
devback
```

### devback init

Global initialization: creates `~/.config/devback/config.toml`, installs hook templates
to `~/.local/share/devback/templates/hooks/`, and (by default) sets `git init.templateDir`
to `~/.local/share/devback/templates/`. If `init.templateDir` is already set to a different
path, the command will fail and suggest using `--force` (overwrite) or `--no-gitconfig` (skip).

Flags:
- `--backup-dir PATH` - base backup directory (**required** when creating config or with `--force`; suggested: `~/.local/share/devback/backups`)
- `--force` - overwrite existing config (with a `config.toml.bak.<timestamp>` backup) and foreign `init.templateDir` value
- `--no-gitconfig` - don't modify `~/.gitconfig`
- `--templates-only` - only update hook templates (skip `config.toml` and `gitconfig`)
- `--dry-run` - show planned changes without writing to disk

### devback setup

Repository setup: copies hooks from global templates and sets
`git config backup.enabled=true` (unless `--no-hooks` is used). Optionally sets `backup.slug`. Requires a prior
`devback init` (hook templates must exist).

Flags:
- `--slug NAME` - set `backup.slug` (for worktree, writes to `config.worktree`)
- `--force` - overwrite existing hooks (main repository only)
- `--no-hooks` - skip hook installation and hook-related git config changes
- `--dry-run` - show planned changes without writing to disk

Notes:
- `--force` is not available for worktrees; hooks are installed only from the main repository
- if hooks are not installed in the main repository, `devback setup` in a worktree will fail
  (or only warn with `--no-hooks`)
- `--no-hooks` does not enable `backup.enabled` and does not modify existing hooks
- if hook files already exist and `--force` is not used, DevBack merges them by creating a backup
  like `post-commit.devback.orig` (or with numeric suffix) and installing a wrapper that runs the original
  hook first and `devback hook <name>` second. The original hook exit code takes priority.

### devback status

Shows global configuration and current repository status. Outside a repository, only the
global section is displayed.

Flags:
- `--no-repo` - show only global configuration
- `--scan-backups` - scan backups to count snapshots/size (may be slow)
- `--dry-run` - accepted for CLI consistency, does not change behavior

### devback

Manual backup using `backup.base_dir` from `config.toml`.
Positional arguments are not supported.

Flags:
- `-v`, `--verbose` - verbose output
- `--dry-run` - full simulation without filesystem changes
- `--print-repo-key` - print the repository key and exit
- `--test-locks` - test the locking mechanism and exit (does not require `backup.base_dir`)

## Configuration

### config.toml (Global Configuration)

Location: `~/.config/devback/config.toml`. Created by `devback init`
and used by `init`, `setup`, `status` commands (paths support `~` and `$HOME`).
If the file is missing, defaults are used and `status` will show `(not found)` for the config.

Configuration example:

```toml
[backup]
base_dir = "~/.local/share/devback/backups"
keep_count = 30
keep_days = 90
max_total_gb = 10
size_margin_mb = 0
no_size = true

[notifications]
enabled = true
sound = "default"

[logging]
dir = "~/.local/state/devback/logs"
level = "info"

[repo_key]
style = "auto"
auto_remote_merge = false
remote_hash_len = 8
```

#### `[backup]` — Backup Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `base_dir` | string | `""` (empty) | Base directory for snapshots. **Required.** Set via `devback init --backup-dir`. Supports [path expansion](#path-expansion). Suggested: `~/.local/share/devback/backups` |
| `keep_count` | int | `30` | Maximum number of snapshots to keep per repository. Oldest snapshots are removed first. |
| `keep_days` | int | `90` | Maximum snapshot age in days. Snapshots older than this are removed during rotation. |
| `max_total_gb` | int | `10` | Maximum total size (GB) of all snapshots per repository. Ignored when `no_size = true`. |
| `size_margin_mb` | int | `0` | Margin in MB added to `max_total_gb` before triggering size-based rotation. |
| `no_size` | bool | `true` | Disable size-based rotation. When `true`, `max_total_gb` and `size_margin_mb` are ignored. |

#### `[notifications]` — Desktop Notifications

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable desktop notifications after backup completion. |
| `sound` | string | `"default"` | Notification sound name. Use `"default"` for the system default sound. Platform-dependent. |

#### `[logging]` — Logging Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dir` | string | `"~/.local/state/devback/logs"` | Directory for log files. Supports [path expansion](#path-expansion). Created automatically if it does not exist. |
| `level` | string | `"info"` | Minimum log level. One of: `debug`, `info`, `warn`, `error`. |

#### `[repo_key]` — Repository Key Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `style` | string | `"auto"` | Repository key naming style. See [Naming Styles](#naming-styles-repo_keystyle) for details. |
| `auto_remote_merge` | bool | `false` | Merge snapshots from clones with the same `remote.origin.url` into a single directory. |
| `remote_hash_len` | int | `8` | Hash suffix length appended to the directory name in `remote-hierarchy` style. |

### Naming Styles (repo_key.style)

#### auto (default)
Automatic style selection in priority order:
1. `backup.slug` + repository basename
2. `remote-hierarchy` + hash (if `repo_key.auto_remote_merge=false`)
3. `name+hash` (fallback)

`backup.slug` can be set via `devback setup --slug`.

#### name+hash
Format: `basename--8char_hash`
Example: `app--4f7a2d9c`

#### remote-hierarchy
Format: `host/owner/repo`
Example: `github.com/acme/app`

#### custom
Uses `git config backup.slug` + repository basename
Example: `work/acme/prod/app`

`backup.slug` can be set via `devback setup --slug`.

### Per-Repository Configuration (git config)

DevBack stores per-repository settings in git config. These are managed by `devback setup` and read by hooks at runtime.

| Setting | Scope | Set by | Description |
|---------|-------|--------|-------------|
| `backup.enabled` | local / worktree | `devback setup` | Enable or disable backup for this repository. |
| `backup.slug` | local / worktree | `devback setup --slug` | Custom prefix for the repository key (e.g., `company/team`). |

**Lookup priority** for `backup.enabled`: worktree config → local config → global config.

For worktrees, `backup.slug` is written to per-worktree config (`extensions.worktreeConfig` is enabled automatically).

Accepted boolean values: `1`, `true`, `yes`, `on` (case-insensitive) are treated as true; everything else is treated as false.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `NO_COLOR` | When set (any value), disables colored terminal output. Follows the [no-color convention](https://no-color.org/). |
| `TERM=dumb` | Disables colored terminal output. |
| `GIT_REFLOG_ACTION` | Used internally by hooks. When it contains `rebase`, the `post-commit` hook is skipped to avoid duplicate backups (the `post-rewrite` hook handles rebase instead). |

### Path Expansion

All path values in `config.toml` (`base_dir`, `dir`) support the following expansions:

| Pattern | Expansion |
|---------|-----------|
| `~` or `~/path` | User's home directory |
| `$HOME` or `$HOME/path` | User's home directory |
| `${HOME}` or `${HOME}/path` | User's home directory |

Example: `base_dir = "~/backups"` expands to `/home/user/backups`.

### Hook Settings

DevBack installs only three hooks: `post-commit`, `post-merge`, `post-rewrite`.
Templates are located in `~/.local/share/devback/templates/hooks/` and copied to `.git/hooks/`
by `devback setup`. The wrappers are minimal and call the binary path captured during `devback init`:

```sh
#!/bin/sh
DEVBACK="__DEVBACK_BIN__"
[ -x "$DEVBACK" ] || { echo "SKIP: devback not found at $DEVBACK" >&2; exit 0; }
exec "$DEVBACK" hook post-commit "$@"
```

All hook logic is implemented in Go for cross-platform compatibility.

### Hook Behavior

- Hooks always exit with code 0 and never block git operations
- If `backup.enabled=false` in git config, the backup is skipped
- If `config.toml` is missing or `backup.base_dir` is empty, the backup is skipped
- If `git ls-files` fails, the backup ends with a critical error
- If the configured DevBack binary path is not executable, the hook script logs a skip message to `stderr` and exits with code 0

### Manual Execution

For manual testing, use `devback hook <name>`.

## .devbackignore File

Create a `.devbackignore` file in the repository root to exclude files from backup:

```
# Exclude temporary files
*.tmp
*.temp

# Exclude logs
*.log

# Exclude directories
node_modules/
dist/
build/

# Exclude specific files
.env.local
config.local.json
```

By default, `devback init` installs a `.devbackignore` template at
`~/.local/share/devback/repo-templates/devbackignore`. `devback setup` creates
`.devbackignore` in the repository root only if the file is missing and the template exists.

## Backup Rotation

The tool automatically manages snapshot size and count:

1. **By age**: Removes snapshots older than `backup.keep_days` days
2. **By count**: Keeps no more than `backup.keep_count` snapshots
3. **By size**: Removes old snapshots when `backup.max_total_gb` is exceeded

Dry-run is available via `--dry-run` and simulates the entire process including rotation.

## Security

- File locking to prevent conflicts
- `.git` directory existence verification
- Proper copy error handling
- Full operation logging

## Performance

- Parallel file copying (worker count = CPU * 2)
- Efficient handling of large repositories
- Minimal memory usage
- Optimized rotation algorithms

## Logging

- `stderr` for logs (info/warn/error)
- `stdout` for useful output only (e.g., `devback status` or `--print-repo-key`)
- Verbose mode with detailed process information

## Exit Codes

DevBack uses standardized exit codes for integration with automated systems
(for all CLI commands):

| Exit Code | Name | Description |
|-----------|------|-------------|
| 0 | `ExitSuccess` | Successful completion |
| 1 | `ExitCriticalError` | Any critical error (e.g., `.git` directory not found) |
| 2 | `ExitUsageError` | Command-line argument error |
| 76 | `ExitLockBusy` | Could not acquire lock (another process is running) |
| 130 | `ExitInterrupted` | Process interrupted by signal |

### Error Handling

- Any critical error terminates the process with exit code 1
- Recommended for mission-critical repositories and automation

## Compatibility

- **Operating systems**: Linux, macOS
- **Git versions**: Compatible with Git 1.8+
- **File systems**: Supports all major file systems

## Integration Examples

### Cron Job for Automatic Backup

```bash
# Daily backup at 02:00
0 2 * * * /usr/local/bin/devback >> /var/log/backup.log 2>&1
```

### Systemd Service

```ini
[Unit]
Description=Git Repository Backup Service
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/devback -v
User=backup
Group=backup

[Install]
WantedBy=multi-user.target
```

## Troubleshooting

### Common Issues

1. **"not a git repository" error**
   - Make sure you are in the root of a Git repository
   - Check that the `.git` directory exists

2. **Permission issues**
   - Check write permissions on the backup directory
   - Ensure read access to the repository

3. **Insufficient disk space**
   - Decrease `backup.max_total_gb`
   - Decrease `backup.keep_days` for more frequent rotation

### Debugging

```bash
# Verbose output for diagnostics
devback -v

# Print repository key
devback --print-repo-key

# Dry-run backup
devback --dry-run -v

# Check exit code for automation
devback; echo "Exit code: $?"
```

### Monitoring and Automation

```bash
# Monitoring script example
#!/bin/bash
devback
case $? in
    0) echo "SUCCESS: Backup completed" ;;
    76) echo "INFO: Another backup running, skipped" ;;
    1) echo "ERROR: Critical backup failure" ; exit 1 ;;
    *) echo "UNKNOWN: Unexpected exit code $?" ;;
esac
```

## Architecture

DevBack is built on clean architecture principles with clear layer separation:

- **cmd/app**: Entry point and initialization
- **internal/usecase**: Application business logic
- **internal/adapters**: External dependency adapters

All operations support context, structured logging, and standardized exit codes.

## Development

```bash
git clone https://github.com/arumata/devback.git
cd devback

# Tests
go test ./... -count=1

# Linter (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
golangci-lint run

# Formatting (install: go install mvdan.cc/gofumpt@latest)
gofumpt -w .
```

## License

This project is distributed under the license specified in the LICENSE file.

## Support

For support or to report issues, please create an issue in the project repository.
