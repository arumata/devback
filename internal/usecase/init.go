package usecase

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const initBackupTimeFormat = "20060102-150405"

// HookBinaryPlaceholder is the placeholder in hook templates replaced with the actual binary path.
const HookBinaryPlaceholder = "__DEVBACK_BIN__"

//nolint:gochecknoglobals // overridden in tests for deterministic backups.
var initNow = time.Now

// InitOptions describes init behavior.
type InitOptions struct {
	BackupDir     string
	Force         bool
	NoGitConfig   bool
	TemplatesOnly bool
	DryRun        bool
	HomeDir       string
	BinaryPath    string
}

// Init performs global initialization of DevBack.
func Init(ctx context.Context, opts InitOptions, deps *Dependencies, logger *slog.Logger) error {
	if logger == nil {
		panic("logger is required")
	}
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if err := validateInitDependencies(deps); err != nil {
		return err
	}

	homeDir, backupDir, err := normalizeInitInputs(opts)
	if err != nil {
		return err
	}

	paths := buildInitPaths(deps.FileSystem, homeDir)

	cfg := DefaultConfigFile()
	if backupDir != "" {
		cfg.Backup.BaseDir = opts.BackupDir
	}
	templatesDir, err := resolveTemplatesDir(ctx, opts, deps, paths, cfg)
	if err != nil {
		return err
	}
	templatesDir, err = normalizeTemplatesDir(templatesDir, homeDir)
	if err != nil {
		return err
	}

	if !opts.TemplatesOnly {
		if err := ensureInitDirs(ctx, deps, homeDir, cfg, opts.DryRun); err != nil {
			return err
		}
	}

	if err := installAllTemplates(ctx, deps, homeDir, templatesDir, opts, logger); err != nil {
		return err
	}

	logger.InfoContext(ctx, "Init completed")
	return nil
}

type initPaths struct {
	configDir  string
	configPath string
}

func validateInitDependencies(deps *Dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are required: %w", ErrCritical)
	}
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available: %w", ErrCritical)
	}
	if deps.Config == nil {
		return fmt.Errorf("config adapter not available: %w", ErrCritical)
	}
	if deps.Templates == nil {
		return fmt.Errorf("templates adapter not available: %w", ErrCritical)
	}
	return nil
}

func normalizeInitInputs(opts InitOptions) (string, string, error) {
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		return "", "", fmt.Errorf("home directory is empty: %w", ErrCritical)
	}
	backupDir := strings.TrimSpace(opts.BackupDir)
	if opts.BackupDir != "" && backupDir == "" {
		return "", "", fmt.Errorf("backup directory is empty: %w", ErrUsage)
	}
	return homeDir, backupDir, nil
}

func buildInitPaths(fs FileSystemPort, homeDir string) initPaths {
	configDir := fs.Join(homeDir, ".config", "devback")
	return initPaths{
		configDir:  configDir,
		configPath: fs.Join(configDir, "config.toml"),
	}
}

func resolveTemplatesDir(
	ctx context.Context,
	opts InitOptions,
	deps *Dependencies,
	paths initPaths,
	cfg ConfigFile,
) (string, error) {
	if opts.TemplatesOnly {
		return DefaultTemplatesDir(), nil
	}
	if err := ensureConfig(ctx, opts, deps, paths, cfg); err != nil {
		return "", err
	}
	return DefaultTemplatesDir(), nil
}

func ensureConfig(ctx context.Context, opts InitOptions, deps *Dependencies, paths initPaths, cfg ConfigFile) error {
	exists, err := pathExists(ctx, deps.FileSystem, paths.configPath)
	if err != nil {
		return fmt.Errorf("check config path: %w", ErrCritical)
	}
	if !exists {
		if err := requireBackupDir(cfg); err != nil {
			return err
		}
		if opts.DryRun {
			return nil
		}
		return writeConfig(ctx, deps, paths, cfg)
	}
	info, err := deps.FileSystem.Stat(ctx, paths.configPath)
	if err != nil {
		return fmt.Errorf("stat config: %w", ErrCritical)
	}
	if info.IsDir() {
		return fmt.Errorf("config path is a directory: %w", ErrUsage)
	}
	if !opts.Force {
		return fmt.Errorf("config already exists at %s: %w", paths.configPath, ErrUsage)
	}
	if err := requireBackupDir(cfg); err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}
	if err := backupConfig(ctx, deps.FileSystem, paths.configPath); err != nil {
		return err
	}
	return writeConfig(ctx, deps, paths, cfg)
}

func requireBackupDir(cfg ConfigFile) error {
	if strings.TrimSpace(cfg.Backup.BaseDir) == "" {
		return fmt.Errorf(
			"--backup-dir flag is required (suggested: %s): %w",
			SuggestedBackupDir, ErrUsage,
		)
	}
	return nil
}

func backupConfig(ctx context.Context, fs FileSystemPort, configPath string) error {
	backupPath := configPath + ".bak." + initNow().Format(initBackupTimeFormat)
	if err := fs.Move(ctx, configPath, backupPath); err != nil {
		return fmt.Errorf("backup config: %w", ErrCritical)
	}
	return nil
}

func writeConfig(ctx context.Context, deps *Dependencies, paths initPaths, cfg ConfigFile) error {
	if err := deps.FileSystem.CreateDir(ctx, paths.configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", ErrCritical)
	}
	if err := deps.Config.Save(ctx, paths.configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", ErrCritical)
	}
	return nil
}

func ensureInitDirs(ctx context.Context, deps *Dependencies, homeDir string, cfg ConfigFile, dryRun bool) error {
	if dryRun {
		return nil
	}
	if dir := strings.TrimSpace(cfg.Backup.BaseDir); dir != "" {
		expanded := normalizePath(deps.FileSystem, dir, homeDir)
		if err := deps.FileSystem.CreateDir(ctx, expanded, 0o755); err != nil {
			return fmt.Errorf("create backup directory: %w", ErrCritical)
		}
	}
	if dir := strings.TrimSpace(cfg.Logging.Dir); dir != "" {
		expanded := normalizePath(deps.FileSystem, dir, homeDir)
		if err := deps.FileSystem.CreateDir(ctx, expanded, 0o755); err != nil {
			return fmt.Errorf("create log directory: %w", ErrCritical)
		}
	}
	return nil
}

func normalizeTemplatesDir(path, homeDir string) (string, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return "", fmt.Errorf("templates directory is empty: %w", ErrCritical)
	}
	expanded := expandHomeDir(clean, homeDir)
	if strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("templates directory is empty: %w", ErrCritical)
	}
	return expanded, nil
}

func shouldConfigureGit(opts InitOptions) bool {
	return !opts.TemplatesOnly && !opts.NoGitConfig
}

func installAllTemplates(
	ctx context.Context, deps *Dependencies, homeDir, templatesDir string, opts InitOptions, logger *slog.Logger,
) error {
	if err := installTemplates(ctx, deps, templatesDir, opts.BinaryPath, opts.DryRun); err != nil {
		return err
	}
	removeLegacyFiles(ctx, deps.FileSystem, templatesDir, opts.DryRun, logger, "template")

	repoTemplatesDir, err := normalizeTemplatesDir(DefaultRepoTemplatesDir(), homeDir)
	if err != nil {
		return err
	}
	if err := installRepoTemplates(ctx, deps, repoTemplatesDir, opts.DryRun); err != nil {
		return err
	}
	removeLegacyRepoTemplatesDir(ctx, deps.FileSystem, templatesDir, opts.DryRun, logger)

	if shouldConfigureGit(opts) {
		if deps.Git == nil {
			return fmt.Errorf("git adapter not available: %w", ErrCritical)
		}
		templateRoot := deps.FileSystem.Dir(templatesDir)
		return ensureGitTemplateDir(
			ctx, deps.FileSystem, deps.Git, homeDir, templateRoot, opts.Force, opts.DryRun,
		)
	}

	return nil
}

func installTemplates(ctx context.Context, deps *Dependencies, hooksDir, binaryPath string, dryRun bool) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	entries, err := deps.Templates.List(ctx)
	if err != nil {
		return fmt.Errorf("list templates: %w", ErrCritical)
	}
	if !dryRun {
		if err := deps.FileSystem.CreateDir(ctx, hooksDir, 0o755); err != nil {
			return fmt.Errorf("create templates dir: %w", ErrCritical)
		}
	}

	placeholder := []byte(HookBinaryPlaceholder)
	binPath := []byte(binaryPath)

	for _, entry := range entries {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		data, err := deps.Templates.Read(ctx, entry.Name)
		if err != nil {
			return fmt.Errorf("read template %s: %w", entry.Name, ErrCritical)
		}
		data = bytes.ReplaceAll(data, placeholder, binPath)
		if dryRun {
			continue
		}
		dest := deps.FileSystem.Join(hooksDir, entry.Name)
		if err := deps.FileSystem.WriteFile(ctx, dest, data, entry.Mode); err != nil {
			return fmt.Errorf("write template %s: %w", entry.Name, ErrCritical)
		}
		if err := deps.FileSystem.Chmod(ctx, dest, entry.Mode); err != nil {
			return fmt.Errorf("chmod template %s: %w", entry.Name, ErrCritical)
		}
	}

	return nil
}

func ensureGitTemplateDir(
	ctx context.Context, fs FileSystemPort, git GitPort,
	homeDir, expected string, force, dryRun bool,
) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	current, err := git.ConfigGetGlobal(ctx, "init.templateDir")
	if err != nil {
		return fmt.Errorf("read git config init.templateDir: %w", ErrCritical)
	}
	currentNorm := normalizePath(fs, current, homeDir)
	expectedNorm := normalizePath(fs, expected, homeDir)

	if currentNorm == "" {
		if dryRun {
			return nil
		}
		if err := git.ConfigSetGlobal(ctx, "init.templateDir", expected); err != nil {
			return fmt.Errorf("set git config init.templateDir: %w", ErrCritical)
		}
		return nil
	}

	if currentNorm == expectedNorm {
		return nil
	}

	if !force {
		return fmt.Errorf(
			"git config init.templateDir already set to %q (expected %q).\n"+
				"Use:\n"+
				"  devback init --force\n"+
				"  devback init --no-gitconfig\n"+
				"  git config --global init.templateDir %s && devback init: %w",
			strings.TrimSpace(current),
			expected,
			expected,
			ErrUsage,
		)
	}

	if dryRun {
		return nil
	}
	if err := git.ConfigSetGlobal(ctx, "init.templateDir", expected); err != nil {
		return fmt.Errorf("set git config init.templateDir: %w", ErrCritical)
	}
	return nil
}

func pathExists(ctx context.Context, fs FileSystemPort, path string) (bool, error) {
	info, err := fs.Stat(ctx, path)
	if err != nil {
		if fs.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info != nil, nil
}

// ExpandHomeDirPublic expands ~ and $HOME prefixes in path.
func ExpandHomeDirPublic(path, homeDir string) string {
	return expandHomeDir(path, homeDir)
}

func expandHomeDir(path, homeDir string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return clean
	}
	if clean == "~" {
		return homeDir
	}
	if strings.HasPrefix(clean, "~/") {
		return strings.TrimRight(homeDir, "/") + clean[1:]
	}
	if clean == "$HOME" {
		return homeDir
	}
	if strings.HasPrefix(clean, "$HOME/") {
		return strings.TrimRight(homeDir, "/") + clean[len("$HOME"):]
	}
	if clean == "${HOME}" {
		return homeDir
	}
	if strings.HasPrefix(clean, "${HOME}/") {
		return strings.TrimRight(homeDir, "/") + clean[len("${HOME}"):]
	}
	return clean
}

func contractHomeDir(path, homeDir string, sep byte) string {
	if homeDir == "" || path == "" {
		return path
	}
	if path == homeDir {
		return "~"
	}
	prefix := homeDir + string(sep)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(sep) + path[len(prefix):]
	}
	return path
}

func normalizePath(fs FileSystemPort, path, homeDir string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	expanded := expandHomeDir(clean, homeDir)
	cleaned := fs.Clean(expanded)
	if cleaned == "." {
		return ""
	}
	return trimTrailingSeparators(fs, cleaned)
}

func trimTrailingSeparators(fs FileSystemPort, path string) string {
	if path == "" {
		return ""
	}
	sep := fs.PathSeparator()
	if path == string(sep) {
		return path
	}
	volume := fs.VolumeName(path)
	if volume != "" {
		rest := strings.TrimPrefix(path, volume)
		if rest == "" || rest == string(sep) || rest == "/" || rest == "\\" {
			return volume + string(sep)
		}
	}
	return strings.TrimRight(path, "/\\")
}

func installRepoTemplates(ctx context.Context, deps *Dependencies, repoTemplatesDir string, dryRun bool) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	entries, err := deps.Templates.ListRepo(ctx)
	if err != nil {
		return fmt.Errorf("list repo templates: %w", ErrCritical)
	}
	if !dryRun {
		if err := deps.FileSystem.CreateDir(ctx, repoTemplatesDir, 0o755); err != nil {
			return fmt.Errorf("create repo templates dir: %w", ErrCritical)
		}
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		data, err := deps.Templates.ReadRepo(ctx, entry.Name)
		if err != nil {
			return fmt.Errorf("read repo template %s: %w", entry.Name, ErrCritical)
		}
		if dryRun {
			continue
		}
		dest := deps.FileSystem.Join(repoTemplatesDir, entry.Name)
		if err := deps.FileSystem.WriteFile(ctx, dest, data, entry.Mode); err != nil {
			return fmt.Errorf("write repo template %s: %w", entry.Name, ErrCritical)
		}
	}

	return nil
}

func legacyHookFiles() []string {
	return []string{
		"_run-backup",
		"hooks-config",
		"hooks-lib.sh",
		"post-checkout",
		"post-rebase",
		"pre-commit",
		"pre-rebase",
		"prepare-commit-msg",
	}
}

func removeLegacyFiles(
	ctx context.Context, fs FileSystemPort, dir string, dryRun bool, logger *slog.Logger, label string,
) {
	if dryRun {
		return
	}
	for _, name := range legacyHookFiles() {
		if ctx.Err() != nil {
			return
		}
		target := fs.Join(dir, name)
		exists, err := pathExists(ctx, fs, target)
		if err != nil || !exists {
			continue
		}
		if err := fs.RemoveAll(ctx, target); err != nil {
			logger.WarnContext(ctx, "Failed to remove legacy "+label, "file", name, "error", err)
			continue
		}
		logger.InfoContext(ctx, "Removed legacy "+label, "file", name)
	}
}

func removeLegacyRepoTemplatesDir(
	ctx context.Context, fs FileSystemPort, templatesDir string, dryRun bool, logger *slog.Logger,
) {
	if dryRun {
		return
	}
	legacyDir := fs.Join(fs.Dir(templatesDir), "repo")
	exists, err := pathExists(ctx, fs, legacyDir)
	if err != nil || !exists {
		return
	}
	if err := fs.RemoveAll(ctx, legacyDir); err != nil {
		logger.WarnContext(ctx, "Failed to remove legacy repo templates dir", "path", legacyDir, "error", err)
		return
	}
	logger.InfoContext(ctx, "Removed legacy repo templates dir", "path", legacyDir)
}
