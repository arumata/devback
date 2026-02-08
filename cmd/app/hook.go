package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

type hookConfig struct {
	verbose  bool
	dryRun   bool
	noNotify bool
}

type hookPreflight struct {
	deps       *usecase.Dependencies
	logger     *slog.Logger
	repoRoot   string
	gitDir     string
	configFile usecase.ConfigFile
	runtimeCfg *usecase.Config
	cleanup    func()
}

func newHookCmd(depsFactory func(*slog.Logger) *usecase.Dependencies, exitCode *int) *cobra.Command {
	cfg := &hookConfig{}
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Git hook commands (called by git hooks)",
		Long: `Git hook commands for automatic backup.

These commands are called by git hooks installed by 'devback setup'.
They can also be run manually for testing.

Hooks always exit with code 0 to never block git operations.`,
	}

	cmd.PersistentFlags().BoolVarP(&cfg.verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().BoolVar(&cfg.dryRun, "dry-run", false, "dry-run mode")
	cmd.PersistentFlags().BoolVar(&cfg.noNotify, "no-notify", false, "disable notifications")

	cmd.AddCommand(newHookPostCommitCmd(cfg, depsFactory, exitCode))
	cmd.AddCommand(newHookPostMergeCmd(cfg, depsFactory, exitCode))
	cmd.AddCommand(newHookPostRewriteCmd(cfg, depsFactory, exitCode))

	return cmd
}

func runHookPreflight(
	ctx context.Context,
	cfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
) (*hookPreflight, bool) {
	logger := setupLogger(cfg.verbose)
	deps := depsFactory(logger)
	if deps == nil || deps.Git == nil || deps.Config == nil || deps.FileSystem == nil {
		return nil, false
	}

	repoRoot, err := deps.Git.RepoRoot(ctx)
	if err != nil {
		logHookSkip(logger, "SKIP_NOT_GIT_REPO")
		return nil, false
	}

	if !readBackupEnabled(ctx, deps.Git, repoRoot) {
		logHookSkip(logger, "SKIP_DISABLED")
		return nil, false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, false
	}

	configFile, configExists, err := loadConfigFile(ctx, deps, homeDir)
	if err != nil {
		return nil, false
	}
	if !configExists {
		logHookSkip(logger, "SKIP_NO_CONFIG")
		return nil, false
	}

	runtimeCfg, err := usecase.RuntimeConfigFromFile(configFile, homeDir)
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(runtimeCfg.BackupDir) == "" {
		logHookSkip(logger, "SKIP_NO_BASEDIR")
		return nil, false
	}

	gitDir, err := deps.Git.GitDir(ctx, repoRoot)
	if err != nil {
		logHookSkip(logger, "SKIP_NOT_GIT_REPO")
		return nil, false
	}
	gitDir = normalizeGitDir(repoRoot, gitDir)

	fileLogger, cleanup := withFileLogging(logger, configFile.Logging, cfg.verbose)

	return &hookPreflight{
		deps:       deps,
		logger:     fileLogger,
		repoRoot:   repoRoot,
		gitDir:     gitDir,
		configFile: configFile,
		runtimeCfg: runtimeCfg,
		cleanup:    cleanup,
	}, true
}

func readBackupEnabled(ctx context.Context, git usecase.GitPort, repoRoot string) bool {
	value := readRepoConfig(ctx, git, repoRoot)
	if value == "" {
		return false
	}
	return parseHookBool(value)
}

func readRepoConfig(ctx context.Context, git usecase.GitPort, repoRoot string) string {
	if git == nil {
		return ""
	}
	if value, err := git.ConfigGetWorktree(ctx, repoRoot, "backup.enabled"); err == nil {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	if value, err := git.ConfigGet(ctx, repoRoot, "backup.enabled"); err == nil {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	if value, err := git.ConfigGetGlobal(ctx, "backup.enabled"); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func parseHookBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeGitDir(repoRoot, gitDir string) string {
	if gitDir == "" {
		return gitDir
	}
	if filepath.IsAbs(gitDir) {
		return filepath.Clean(gitDir)
	}
	return filepath.Clean(filepath.Join(repoRoot, gitDir))
}

func logHookSkip(logger *slog.Logger, reason string) {
	if logger == nil {
		return
	}
	logger.Debug("skip hook", "reason", reason)
}

// isRebaseInProgress checks for rebase state files/dirs.
func isRebaseInProgress(ctx context.Context, fs usecase.FileSystemPort, gitDir string) (bool, error) {
	if fs == nil {
		return false, errors.New("filesystem dependency is missing")
	}
	if strings.TrimSpace(gitDir) == "" {
		return false, errors.New("git dir is empty")
	}

	rebaseMerge := fs.Join(gitDir, "rebase-merge")
	rebaseApply := fs.Join(gitDir, "rebase-apply")
	rebaseHead := fs.Join(gitDir, "REBASE_HEAD")

	exists, err := hookPathExists(ctx, fs, rebaseMerge)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	exists, err = hookPathExists(ctx, fs, rebaseApply)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	exists, err = hookPathExists(ctx, fs, rebaseHead)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func hookPathExists(ctx context.Context, fs usecase.FileSystemPort, path string) (bool, error) {
	info, err := fs.Stat(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info == nil {
		return false, nil
	}
	return true, nil
}
