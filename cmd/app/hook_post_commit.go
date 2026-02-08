package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

func newHookPostCommitCmd(
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	exitCode *int,
) *cobra.Command {
	return &cobra.Command{
		Use:   "post-commit",
		Short: "Backup after commit",
		Long: `Run backup after commit.

Skips backup if:
- backup.enabled is false in git config
- Rebase is in progress
- Config file is missing or backup.base_dir is empty`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			*exitCode = runHookPostCommit(cmd.Context(), hookCfg, depsFactory)
		},
	}
}

func runHookPostCommit(
	ctx context.Context,
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
) int {
	preflight, ok := runHookPreflight(ctx, hookCfg, depsFactory)
	if !ok || preflight == nil {
		return exitSuccess
	}
	defer preflight.cleanup()
	if ctx.Err() != nil {
		return exitSuccess
	}

	if isRebaseReflogAction() {
		logHookSkip(preflight.logger, "SKIP_REBASE_REFLOG")
		return exitSuccess
	}

	inRebase, err := isRebaseInProgress(ctx, preflight.deps.FileSystem, preflight.gitDir)
	if err != nil {
		return exitSuccess
	}
	if inRebase {
		logHookSkip(preflight.logger, "SKIP_REBASE_IN_PROGRESS")
		return exitSuccess
	}

	return runBackupWithNotify(ctx, hookCfg, preflight)
}

func isRebaseReflogAction() bool {
	action := os.Getenv("GIT_REFLOG_ACTION")
	return strings.Contains(strings.ToLower(action), "rebase")
}

//nolint:unparam // hooks always return exitSuccess to never block git
func runBackupWithNotify(ctx context.Context, hookCfg *hookConfig, preflight *hookPreflight) int {
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
		if errors.Is(err, usecase.ErrLockBusy) || errors.Is(err, usecase.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return exitSuccess
		}
		sendHookNotification(ctx, hookCfg, preflight, false, result)
		return exitSuccess
	}

	sendHookNotification(ctx, hookCfg, preflight, true, result)
	return exitSuccess
}

func sendHookNotification(
	ctx context.Context,
	hookCfg *hookConfig,
	preflight *hookPreflight,
	success bool,
	result *usecase.BackupResult,
) {
	if hookCfg == nil || preflight == nil {
		return
	}
	if hookCfg.noNotify || preflight.deps == nil || preflight.deps.Notification == nil {
		return
	}
	if !preflight.configFile.Notifications.Enabled {
		return
	}

	title := "DevBack"
	repo := shortenHome(preflight.repoRoot)
	sound := "Basso"

	var message string
	switch {
	case !success:
		message = fmt.Sprintf("%s: Backup failed", repo)
	case result != nil && result.PartialSuccess:
		sound = notificationSound(preflight)
		message = fmt.Sprintf("%s: %d files copied, %d errors", repo, result.CopiedFiles, result.SkippedFiles)
	case result != nil:
		sound = notificationSound(preflight)
		message = fmt.Sprintf("%s: %d files copied", repo, result.CopiedFiles)
	default:
		sound = notificationSound(preflight)
		message = fmt.Sprintf("%s: Backup completed", repo)
	}

	_ = preflight.deps.Notification.Send(ctx, title, message, sound)
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	if path == home {
		return "~"
	}
	return path
}

func notificationSound(preflight *hookPreflight) string {
	sound := strings.TrimSpace(preflight.configFile.Notifications.Sound)
	if sound == "" {
		return "default"
	}
	return sound
}
