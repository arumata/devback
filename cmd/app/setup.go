package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

func newSetupCmd(depsFactory func(*slog.Logger) *usecase.Dependencies, exitCode *int) *cobra.Command {
	var (
		slug    string
		force   bool
		noHooks bool
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure current repository for DevBack",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			logger := setupLogger(false)
			deps := depsFactory(logger)
			homeDir, err := os.UserHomeDir()
			if err != nil {
				handleCmdError(exitCode, fmt.Errorf("resolve home dir: %w", usecase.ErrCritical))
				return
			}
			opts := usecase.SetupOptions{
				Slug:    slug,
				Force:   force,
				NoHooks: noHooks,
				DryRun:  dryRun,
				HomeDir: homeDir,
			}
			handleCmdError(exitCode, usecase.Setup(cmd.Context(), opts, deps, logger))
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "set backup.slug (worktree: config.worktree)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing hooks (main repository only)")
	cmd.Flags().BoolVar(&noHooks, "no-hooks", false, "configure git without installing hooks")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan changes without writing to disk")

	_ = cmd.RegisterFlagCompletionFunc("slug",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			dir, err := os.Getwd()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return []string{filepath.Base(dir)}, cobra.ShellCompDirectiveNoFileComp
		},
	)

	return cmd
}
