package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

const (
	stampFileName   = "devback-backup-stamp"
	debounceTimeout = 60 * time.Second
)

func newHookPostRewriteCmd(
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	exitCode *int,
) *cobra.Command {
	return &cobra.Command{
		Use:   "post-rewrite <command>",
		Short: "Backup after rewrite (rebase/amend)",
		Long: `Run backup after rebase or amend.

Deduplicates via a sha-aware debounce (60s): a backup already taken for the
same HEAD (e.g. by post-commit during git commit --amend) is not repeated,
while a new HEAD within the window still gets its own snapshot.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{"rebase", "amend"}, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(cmd *cobra.Command, args []string) {
			*exitCode = runHookPostRewrite(cmd.Context(), hookCfg, depsFactory, args[0])
		},
	}
}

// The rewrite command (rebase/amend) stays in the CLI contract but no longer
// branches the logic: both paths share the sha-aware debounce below.
func runHookPostRewrite(
	ctx context.Context,
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	_ string,
) int {
	preflight, ok := runHookPreflight(ctx, hookCfg, depsFactory)
	if !ok || preflight == nil {
		return exitSuccess
	}
	defer preflight.cleanup()
	if ctx.Err() != nil {
		return exitSuccess
	}

	// No isRebaseInProgress guard here: git invokes post-rewrite when the
	// rewrite has already happened, while the rebase state dir is still on
	// disk — the guard made this path unreachable and a completed rebase
	// produced no snapshot at all. The debounce stamp deduplicates repeated
	// invocations instead, and it applies to amend as well: `git commit
	// --amend` fires post-commit first, whose backup already covers the
	// exact HEAD this hook sees. A stamp left by a different HEAD does not
	// debounce (see isDebounceActive), so a genuinely new rewrite within
	// the window is still backed up.
	stampPath := resolveStampPath(ctx, preflight)
	headSha := readHeadSha(ctx, preflight)
	debounce, err := isDebounceActive(ctx, preflight.deps.FileSystem, stampPath, time.Now(), headSha)
	if err != nil {
		preflight.logger.Debug("failed to read debounce stamp", "error", err)
	}
	if debounce {
		logBackupSkipped(preflight, "post-rewrite", "SKIP_DEBOUNCE")
		return exitSuccess
	}

	return runPostRewriteBackup(ctx, hookCfg, preflight, stampPath)
}

//nolint:unparam // hooks always return exitSuccess to never block git
func runPostRewriteBackup(
	ctx context.Context,
	hookCfg *hookConfig,
	preflight *hookPreflight,
	stampPath string,
) int {
	if hookCfg == nil || preflight == nil {
		return exitSuccess
	}
	if ctx.Err() != nil {
		return exitSuccess
	}
	if hookCfg.dryRun {
		preflight.logger.Info("dry-run: would run backup")
		return exitSuccess
	}

	cfg := &usecase.Config{}
	applyBackupConfig(cfg, preflight.runtimeCfg)
	cfg.BackupDir = preflight.runtimeCfg.BackupDir
	cfg.Verbose = hookCfg.verbose
	cfg.DryRun = hookCfg.dryRun

	result, err := usecase.Backup(ctx, cfg, preflight.deps, preflight.logger)
	if err != nil {
		if errors.Is(err, usecase.ErrLockBusy) {
			logBackupSkipped(preflight, "post-rewrite", "SKIP_LOCK_BUSY")
			return exitSuccess
		}
		if errors.Is(err, usecase.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return exitSuccess
		}
		updateStampWithLog(ctx, preflight, stampPath)
		sendHookNotification(ctx, hookCfg, preflight, false, result)
		return exitSuccess
	}

	updateStampWithLog(ctx, preflight, stampPath)
	sendHookNotification(ctx, hookCfg, preflight, true, result)
	return exitSuccess
}

func resolveStampPath(ctx context.Context, preflight *hookPreflight) string {
	if preflight == nil || preflight.deps == nil || preflight.deps.Git == nil || preflight.deps.FileSystem == nil {
		return ""
	}
	commonDir, err := preflight.deps.Git.GitCommonDir(ctx, preflight.repoRoot)
	if err == nil {
		commonDir = normalizeGitDir(preflight.repoRoot, commonDir)
		if strings.TrimSpace(commonDir) != "" {
			return preflight.deps.FileSystem.Join(commonDir, stampFileName)
		}
	}
	if strings.TrimSpace(preflight.gitDir) == "" {
		return ""
	}
	return preflight.deps.FileSystem.Join(preflight.gitDir, stampFileName)
}

// isDebounceActive reports whether a recent backup already covers the current
// state. The stamp is "unix-timestamp[ head-sha]"; the sha makes the debounce
// state-aware — a stamp left by a different HEAD never debounces, so two
// rebases within the window still produce two snapshots. When either sha is
// unknown the check degrades to time-only.
func isDebounceActive(
	ctx context.Context,
	fs usecase.FileSystemPort,
	stampPath string,
	now time.Time,
	headSha string,
) (bool, error) {
	if fs == nil || strings.TrimSpace(stampPath) == "" {
		return false, nil
	}
	data, err := fs.ReadFile(ctx, stampPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return false, nil
	}
	ts, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return false, nil
	}
	if now.Sub(time.Unix(ts, 0)) >= debounceTimeout {
		return false, nil
	}
	if len(fields) > 1 && headSha != "" && fields[1] != headSha {
		return false, nil
	}
	return true, nil
}

func updateStampWithLog(ctx context.Context, preflight *hookPreflight, stampPath string) {
	if preflight == nil || preflight.deps == nil || preflight.deps.FileSystem == nil {
		return
	}
	if strings.TrimSpace(stampPath) == "" {
		return
	}
	if err := updateStamp(ctx, preflight.deps.FileSystem, stampPath, readHeadSha(ctx, preflight)); err != nil {
		preflight.logger.Debug("failed to update debounce stamp", "error", err)
	}
}

func updateStamp(ctx context.Context, fs usecase.FileSystemPort, stampPath, headSha string) error {
	if fs == nil {
		return errors.New("filesystem dependency is missing")
	}
	if strings.TrimSpace(stampPath) == "" {
		return errors.New("stamp path is empty")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tmpPath := stampPath + ".tmp"
	stamp := fmt.Sprintf("%d", time.Now().Unix())
	if headSha != "" {
		stamp += " " + headSha
	}
	if err := fs.WriteFile(ctx, tmpPath, []byte(stamp), 0o644); err != nil {
		return err
	}
	return fs.Move(ctx, tmpPath, stampPath)
}
