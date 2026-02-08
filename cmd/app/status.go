package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

func newStatusCmd(depsFactory func(*slog.Logger) *usecase.Dependencies, exitCode *int) *cobra.Command {
	var (
		noRepo      bool
		scanBackups bool
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show DevBack configuration and repository status",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			logger := setupLogger(false)
			deps := depsFactory(logger)
			homeDir, err := os.UserHomeDir()
			if err != nil {
				handleCmdError(exitCode, fmt.Errorf("resolve home dir: %w", usecase.ErrCritical))
				return
			}
			opts := usecase.StatusOptions{
				NoRepo:      noRepo,
				ScanBackups: scanBackups,
				DryRun:      dryRun,
				HomeDir:     homeDir,
			}
			report, err := usecase.Status(cmd.Context(), opts, deps, logger)
			if err != nil {
				handleCmdError(exitCode, err)
				return
			}
			if _, err := fmt.Fprint(os.Stdout, usecase.FormatStatus(report, shouldUseColor(os.Stdout))); err != nil {
				handleCmdError(exitCode, err)
				return
			}
			*exitCode = exitSuccess
		},
	}

	cmd.Flags().BoolVar(&noRepo, "no-repo", false, "show only global configuration")
	cmd.Flags().BoolVar(&scanBackups, "scan-backups", false, "scan backups for snapshots and size")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "accept but do not change behavior")

	return cmd
}
