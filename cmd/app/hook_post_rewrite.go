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

For rebase: uses debounce (60s) to run backup only once.
For amend: runs backup immediately.`,
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

func runHookPostRewrite(
	ctx context.Context,
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	command string,
) int {
	preflight, ok := runHookPreflight(ctx, hookCfg, depsFactory)
	if !ok || preflight == nil {
		return exitSuccess
	}
	defer preflight.cleanup()
	if ctx.Err() != nil {
		return exitSuccess
	}

	command = strings.ToLower(strings.TrimSpace(command))
	isRebase := command == "rebase"

	stampPath := resolveStampPath(ctx, preflight)
	if isRebase {
		inRebase, err := isRebaseInProgress(ctx, preflight.deps.FileSystem, preflight.gitDir)
		if err != nil {
			return exitSuccess
		}
		if inRebase {
			logHookSkip(preflight.logger, "SKIP_REBASE_IN_PROGRESS")
			return exitSuccess
		}

		debounce, err := isDebounceActive(ctx, preflight.deps.FileSystem, stampPath, time.Now())
		if err != nil {
			preflight.logger.Debug("failed to read debounce stamp", "error", err)
		}
		if debounce {
			logHookSkip(preflight.logger, "SKIP_DEBOUNCE")
			return exitSuccess
		}
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
			logHookSkip(preflight.logger, "SKIP_LOCK_BUSY")
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

func isDebounceActive(ctx context.Context, fs usecase.FileSystemPort, stampPath string, now time.Time) (bool, error) {
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
	stampValue := strings.TrimSpace(string(data))
	if stampValue == "" {
		return false, nil
	}
	ts, err := strconv.ParseInt(stampValue, 10, 64)
	if err != nil {
		return false, nil
	}
	stampTime := time.Unix(ts, 0)
	return now.Sub(stampTime) < debounceTimeout, nil
}

func updateStampWithLog(ctx context.Context, preflight *hookPreflight, stampPath string) {
	if preflight == nil || preflight.deps == nil || preflight.deps.FileSystem == nil {
		return
	}
	if strings.TrimSpace(stampPath) == "" {
		return
	}
	if err := updateStamp(ctx, preflight.deps.FileSystem, stampPath); err != nil {
		preflight.logger.Debug("failed to update debounce stamp", "error", err)
	}
}

func updateStamp(ctx context.Context, fs usecase.FileSystemPort, stampPath string) error {
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
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	if err := fs.WriteFile(ctx, tmpPath, []byte(timestamp), 0o644); err != nil {
		return err
	}
	return fs.Move(ctx, tmpPath, stampPath)
}
