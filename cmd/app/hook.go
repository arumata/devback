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
	deps          *usecase.Dependencies
	logger        *slog.Logger
	skipLogger    *slog.Logger // file-only, pinned to info: skip reasons must reach the log without touching the terminal
	consoleLogger *slog.Logger // stderr-only, for --verbose echoes that must not duplicate into the file
	repoRoot      string
	gitDir        string
	configFile    usecase.ConfigFile
	runtimeCfg    *usecase.Config
	cleanup       func()
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

	// Past this point the repo is enrolled: a skipped backup is a real loss
	// of protection, so failures must be visible. The file logger is not
	// attached yet, so warn to stderr — a broken config deserves one line
	// per commit until it is fixed.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("skip hook", "reason", "SKIP_NO_HOMEDIR", "repo", repoRoot, "error", err)
		return nil, false
	}

	configFile, configExists, err := loadConfigFile(ctx, deps, homeDir)
	if err != nil {
		logger.Warn("skip hook", "reason", "SKIP_CONFIG_ERROR", "repo", repoRoot, "error", err)
		return nil, false
	}
	if !configExists {
		logger.Warn("skip hook", "reason", "SKIP_NO_CONFIG", "repo", repoRoot)
		return nil, false
	}

	runtimeCfg, err := usecase.RuntimeConfigFromFile(configFile, homeDir)
	if err != nil {
		logger.Warn("skip hook", "reason", "SKIP_CONFIG_ERROR", "repo", repoRoot, "error", err)
		return nil, false
	}
	if strings.TrimSpace(runtimeCfg.BackupDir) == "" {
		logger.Warn("skip hook", "reason", "SKIP_NO_BASEDIR", "repo", repoRoot)
		return nil, false
	}

	gitDir, err := deps.Git.GitDir(ctx, repoRoot)
	if err != nil {
		logger.Warn("skip hook", "reason", "SKIP_NOT_GIT_REPO", "repo", repoRoot, "error", err)
		return nil, false
	}
	gitDir = normalizeGitDir(repoRoot, gitDir)

	fileLogger, fileOnly, cleanup := withFileLogging(logger, configFile.Logging, cfg.verbose)

	return &hookPreflight{
		deps:          deps,
		logger:        fileLogger,
		skipLogger:    fileOnly.With("repo", repoRoot),
		consoleLogger: logger,
		repoRoot:      repoRoot,
		gitDir:        gitDir,
		configFile:    configFile,
		runtimeCfg:    runtimeCfg,
		cleanup:       cleanup,
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

// logHookSkip records skips that happen before enrollment is known
// (not a git repo, backup.enabled unset): these fire on every commit in
// repos where devback is not enabled, so debug keeps the console quiet.
// Enrolled-repo skips go through logBackupSkipped instead.
func logHookSkip(logger *slog.Logger, reason string) {
	if logger == nil {
		return
	}
	logger.Debug("skip hook", "reason", reason)
}

// logBackupSkipped records a skipped backup in an enrolled repo: the reason
// goes to the file log (info, with hook and repo attributes) so a repo that
// silently stopped being backed up can be diagnosed, and to the console only
// at debug so git output stays clean unless --verbose is given. The console
// echo uses the stderr-only logger, otherwise --verbose would write the same
// skip into the file twice.
func logBackupSkipped(p *hookPreflight, hook, reason string) {
	if p == nil {
		return
	}
	if p.skipLogger != nil {
		p.skipLogger.Info("skip hook", "hook", hook, "reason", reason)
	}
	if p.consoleLogger != nil {
		p.consoleLogger.Debug("skip hook", "hook", hook, "reason", reason)
	}
}

// readHeadSha resolves the current HEAD commit sha via the filesystem port:
// gitDir/HEAD directly for a detached head, or the loose ref file under the
// common dir for a symbolic one. Returns "" when it cannot tell (e.g. the
// ref is packed) — callers must degrade gracefully.
func readHeadSha(ctx context.Context, p *hookPreflight) string {
	if p == nil || p.deps == nil || p.deps.FileSystem == nil || strings.TrimSpace(p.gitDir) == "" {
		return ""
	}
	fs := p.deps.FileSystem
	data, err := fs.ReadFile(ctx, fs.Join(p.gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	if !strings.HasPrefix(head, "ref:") {
		return head
	}
	refPath := strings.TrimSpace(strings.TrimPrefix(head, "ref:"))
	if refPath == "" {
		return ""
	}
	refDir := p.gitDir
	if p.deps.Git != nil {
		if commonDir, err := p.deps.Git.GitCommonDir(ctx, p.repoRoot); err == nil {
			if normalized := normalizeGitDir(p.repoRoot, commonDir); strings.TrimSpace(normalized) != "" {
				refDir = normalized
			}
		}
	}
	data, err = fs.ReadFile(ctx, fs.Join(refDir, refPath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// isRebaseInProgress checks for the rebase state dirs rebase-merge and
// rebase-apply. A regular file with those names does not count, and
// REBASE_HEAD is deliberately ignored — git can leave it behind after a
// finished or aborted rebase, and treating it as active silently disabled
// backups until it was removed by hand.
func isRebaseInProgress(ctx context.Context, fs usecase.FileSystemPort, gitDir string) (bool, error) {
	if fs == nil {
		return false, errors.New("filesystem dependency is missing")
	}
	if strings.TrimSpace(gitDir) == "" {
		return false, errors.New("git dir is empty")
	}

	rebaseMerge := fs.Join(gitDir, "rebase-merge")
	rebaseApply := fs.Join(gitDir, "rebase-apply")

	exists, err := hookDirExists(ctx, fs, rebaseMerge)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	return hookDirExists(ctx, fs, rebaseApply)
}

func hookDirExists(ctx context.Context, fs usecase.FileSystemPort, path string) (bool, error) {
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
	return info.IsDir(), nil
}
