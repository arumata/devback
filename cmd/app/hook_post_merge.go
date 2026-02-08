package main

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

func newHookPostMergeCmd(
	hookCfg *hookConfig,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	exitCode *int,
) *cobra.Command {
	return &cobra.Command{
		Use:   "post-merge",
		Short: "Backup after merge",
		Long: `Run backup after merge.

Skips backup if:
- backup.enabled is false in git config
- Rebase is in progress
- Config file is missing or backup.base_dir is empty`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			*exitCode = runHookPostMerge(cmd.Context(), hookCfg, depsFactory)
		},
	}
}

func runHookPostMerge(
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
