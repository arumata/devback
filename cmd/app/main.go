package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/arumata/devback/internal/adapters/loghandler"
	"github.com/arumata/devback/internal/app"
	"github.com/arumata/devback/internal/usecase"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	)
	defer stop()

	cfg := &usecase.Config{}

	cmd, exitCode := newRootCmd(
		cfg,
		app.NewDefaultDependencies,
		func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) error {
			_, err := usecase.Backup(ctx, cfg, deps, logger)
			return err
		},
		func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) error {
			return usecase.TestLocks(ctx, cfg, deps, logger)
		},
		func(cfg *usecase.Config, deps *usecase.Dependencies, logger *slog.Logger) (string, error) {
			return usecase.PrintRepoKey(ctx, cfg, deps, logger)
		},
	)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsageError
	}
	return *exitCode
}

func newRootCmd(
	cfg *usecase.Config,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	run func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	testLocks func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	printRepoKey func(*usecase.Config, *usecase.Dependencies, *slog.Logger) (string, error),
) (*cobra.Command, *int) {
	exitCode := 0
	cmd := &cobra.Command{
		Use:           "devback",
		SilenceUsage:  false,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitCode = runRootCommand(cmd, cfg, depsFactory, run, testLocks, printRepoKey)
		},
	}
	cmd.SetErr(os.Stderr)

	cmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "verbose output")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "full dry-run (no filesystem changes)")
	cmd.Flags().BoolVar(&cfg.PrintRepoKey, "print-repo-key", false, "print repository key and exit (no backup)")
	cmd.Flags().BoolVar(&cfg.TestLocks, "test-locks", false, "test enhanced lock system and exit")

	cmd.AddCommand(newInitCmd(depsFactory, &exitCode))
	cmd.AddCommand(newSetupCmd(depsFactory, &exitCode))
	cmd.AddCommand(newStatusCmd(depsFactory, &exitCode))
	cmd.AddCommand(newHookCmd(depsFactory, &exitCode))
	cmd.AddCommand(newVersionCmd())

	return cmd, &exitCode
}

func runRootCommand(
	cmd *cobra.Command,
	cfg *usecase.Config,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	run func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	testLocks func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	printRepoKey func(*usecase.Config, *usecase.Dependencies, *slog.Logger) (string, error),
) int {
	logger := setupLogger(cfg.Verbose)

	state, err := initRootState(cmd.Context(), depsFactory, logger)
	if err != nil {
		return mapExitCodeWithLog(err)
	}
	if state.configExists {
		applyBackupConfig(cfg, state.backupCfg)
	}
	fileLogger, cleanup := withFileLogging(logger, state.configFile.Logging, cfg.Verbose)
	defer cleanup()
	logger = fileLogger
	logger.Info("Starting devback application")
	if !cfg.TestLocks && !cfg.PrintRepoKey && cfg.BackupDir == "" {
		fmt.Fprintln(os.Stderr, "backup.base_dir not configured (run: devback init --backup-dir <path>)")
		return exitUsageError
	}
	return executeRootAction(cfg, state.deps, logger, run, testLocks, printRepoKey)
}

type rootState struct {
	deps         *usecase.Dependencies
	configFile   usecase.ConfigFile
	backupCfg    *usecase.Config
	configExists bool
}

func initRootState(
	ctx context.Context,
	depsFactory func(*slog.Logger) *usecase.Dependencies,
	logger *slog.Logger,
) (rootState, error) {
	deps := depsFactory(logger)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return rootState{}, fmt.Errorf("resolve home dir: %v: %w", err, usecase.ErrCritical)
	}
	configFile, configExists, err := loadConfigFile(ctx, deps, homeDir)
	if err != nil {
		return rootState{}, err
	}
	backupCfg, err := usecase.RuntimeConfigFromFile(configFile, homeDir)
	if err != nil {
		return rootState{}, err
	}
	return rootState{
		deps:         deps,
		configFile:   configFile,
		backupCfg:    backupCfg,
		configExists: configExists,
	}, nil
}

func executeRootAction(
	cfg *usecase.Config,
	deps *usecase.Dependencies,
	logger *slog.Logger,
	run func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	testLocks func(*usecase.Config, *usecase.Dependencies, *slog.Logger) error,
	printRepoKey func(*usecase.Config, *usecase.Dependencies, *slog.Logger) (string, error),
) int {
	if cfg.TestLocks {
		return mapExitCodeWithLog(testLocks(cfg, deps, logger))
	}
	if cfg.PrintRepoKey {
		repoKey, err := printRepoKey(cfg, deps, logger)
		if err != nil {
			return mapExitCodeWithLog(err)
		}
		if _, err := fmt.Fprintln(os.Stdout, repoKey); err != nil {
			return mapExitCodeWithLog(err)
		}
		return exitSuccess
	}
	return mapExitCodeWithLog(run(cfg, deps, logger))
}

func mapExitCode(err error) int {
	if err == nil {
		return exitSuccess
	}
	switch {
	case errors.Is(err, usecase.ErrUsage):
		return exitUsageError
	case errors.Is(err, usecase.ErrLockBusy):
		return exitLockBusy
	case errors.Is(err, usecase.ErrInterrupted):
		return exitInterrupted
	default:
		return exitCriticalError
	}
}

func loadConfigFile(
	ctx context.Context,
	deps *usecase.Dependencies,
	homeDir string,
) (usecase.ConfigFile, bool, error) {
	if deps == nil || deps.Config == nil || deps.FileSystem == nil {
		return usecase.ConfigFile{}, false, fmt.Errorf("dependencies not available: %w", usecase.ErrCritical)
	}
	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	info, err := deps.FileSystem.Stat(ctx, configPath)
	exists := false
	if err == nil {
		if info != nil && info.IsDir() {
			return usecase.ConfigFile{}, false, fmt.Errorf("config path is a directory: %w", usecase.ErrUsage)
		}
		exists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return usecase.ConfigFile{}, false, fmt.Errorf("stat config: %w", usecase.ErrCritical)
	}
	cfg, err := deps.Config.Load(ctx, configPath)
	if err != nil {
		return usecase.ConfigFile{}, false, fmt.Errorf("load config: %w", usecase.ErrCritical)
	}
	return cfg, exists, nil
}

func applyBackupConfig(target, source *usecase.Config) {
	if target == nil || source == nil {
		return
	}
	target.BackupDir = source.BackupDir
	target.KeepCount = source.KeepCount
	target.KeepDays = source.KeepDays
	target.MaxTotalGBPerRepo = source.MaxTotalGBPerRepo
	target.SizeMarginMB = source.SizeMarginMB
	target.RepoKeyStyle = source.RepoKeyStyle
	target.AutoRemoteMerge = source.AutoRemoteMerge
	target.RemoteHashLen = source.RemoteHashLen
	target.NoSize = source.NoSize
}

func setupLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	handler := loghandler.NewHandler(os.Stderr, &loghandler.Options{
		Level:    level,
		UseColor: shouldUseColor(os.Stderr),
	})
	return slog.New(handler)
}

func withFileLogging(
	logger *slog.Logger,
	logCfg usecase.LoggingConfig,
	verbose bool,
) (*slog.Logger, func()) {
	dir := strings.TrimSpace(logCfg.Dir)
	if dir == "" {
		return logger, func() {}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("Cannot resolve home dir for log file", "error", err)
		return logger, func() {}
	}
	expanded := usecase.ExpandHomeDirPublic(dir, homeDir)
	if err := os.MkdirAll(expanded, 0o750); err != nil {
		logger.Warn("Cannot create log directory", "path", expanded, "error", err)
		return logger, func() {}
	}
	filename := "devback-" + time.Now().Format("2006-01-02") + ".log"
	logPath := filepath.Join(expanded, filename)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // path from config
	if err != nil {
		logger.Warn("Cannot open log file", "path", logPath, "error", err)
		return logger, func() {}
	}

	fileLevel := parseLogLevel(logCfg.Level)
	if verbose && fileLevel > slog.LevelDebug {
		fileLevel = slog.LevelDebug
	}
	fileHandler := loghandler.NewHandler(f, &loghandler.Options{
		Level:    fileLevel,
		UseColor: false,
	})

	stderrHandler := logger.Handler()
	combined := loghandler.NewMultiHandler(stderrHandler, fileHandler)
	return slog.New(combined), func() { _ = f.Close() }
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func shouldUseColor(f *os.File) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
