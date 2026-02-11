package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/usecase"
)

func newInitCmd(depsFactory func(*slog.Logger) *usecase.Dependencies, exitCode *int) *cobra.Command {
	var (
		backupDir     string
		force         bool
		noGitConfig   bool
		templatesOnly bool
		dryRun        bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DevBack",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			logger := setupLogger(false)
			deps := depsFactory(logger)
			homeDir, err := os.UserHomeDir()
			if err != nil {
				handleCmdError(exitCode, fmt.Errorf("resolve home dir: %w", usecase.ErrCritical))
				return
			}
			exePath, err := os.Executable()
			if err != nil {
				handleCmdError(exitCode, fmt.Errorf("resolve executable path: %w", usecase.ErrCritical))
				return
			}
			opts := usecase.InitOptions{
				BackupDir:     backupDir,
				Force:         force,
				NoGitConfig:   noGitConfig,
				TemplatesOnly: templatesOnly,
				DryRun:        dryRun,
				HomeDir:       homeDir,
				BinaryPath:    filepath.Clean(exePath),
			}
			handleCmdError(exitCode, usecase.Init(cmd.Context(), opts, deps, logger))
		},
	}

	cmd.Flags().StringVar(
		&backupDir, "backup-dir", "",
		"backup base directory (suggested: ~/.local/share/devback/backups)",
	)
	cmd.Flags().BoolVar(
		&force, "force", false,
		"overwrite existing config and git init.templateDir (with backup for config)",
	)
	cmd.Flags().BoolVar(&noGitConfig, "no-gitconfig", false, "skip global git config change")
	cmd.Flags().BoolVar(&templatesOnly, "templates-only", false, "install/update templates only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan changes without writing to disk")

	_ = cmd.RegisterFlagCompletionFunc("backup-dir",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
	)

	return cmd
}
