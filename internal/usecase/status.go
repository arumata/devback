package usecase

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"
)

const statusNotSet = "(not set)"

// statusPalette holds ANSI escape sequences for colorized status output.
// When useColor is false, all fields are empty strings (no-op coloring).
type statusPalette struct {
	reset    string
	bold     string
	dim      string
	green    string
	red      string
	yellow   string
	cyan     string
	boldCyan string
}

func newStatusPalette(useColor bool) statusPalette {
	if !useColor {
		return statusPalette{}
	}
	return statusPalette{
		reset:    "\033[0m",
		bold:     "\033[1m",
		dim:      "\033[2m",
		green:    "\033[32m",
		red:      "\033[31m",
		yellow:   "\033[33m",
		cyan:     "\033[36m",
		boldCyan: "\033[1;36m",
	}
}

// StatusOptions describes status output behavior.
type StatusOptions struct {
	NoRepo      bool
	ScanBackups bool
	DryRun      bool
	HomeDir     string
}

// StatusReport contains status information for rendering.
type StatusReport struct {
	Global    StatusGlobal
	Repo      *StatusRepo
	Worktrees []WorktreeInfo
}

// StatusGlobal contains global configuration checks.
type StatusGlobal struct {
	ConfigFile     StatusPath
	TemplatesDir   StatusPath
	BackupBase     StatusPath
	LogDir         StatusPath
	GitTemplateDir StatusGitTemplateDir
}

// StatusPath describes a path and its availability.
type StatusPath struct {
	Path   string
	Exists bool
	Source string
}

// StatusGitTemplateDir describes the git templateDir status.
type StatusGitTemplateDir struct {
	Expected string
	Actual   string
	Set      bool
	Matches  bool
	Hint     string
}

// StatusRepo contains repository-specific status information.
type StatusRepo struct {
	Root          string
	Type          RepoType
	Branch        string
	MainRoot      string
	Hooks         StatusHooks
	BackupEnabled bool
	BackupSlug    string
	RepoKey       string
	Backups       StatusBackups
}

// StatusHooks contains hook checks summary.
type StatusHooks struct {
	Installed  int
	Executable int
	Total      int
	Current    StatusCurrent
}

// StatusCurrent describes hooks current status.
type StatusCurrent struct {
	Known   bool
	Matches bool
}

// StatusBackups contains backup scan results.
type StatusBackups struct {
	Scanned       bool
	SnapshotCount int
	TotalSizeKB   int64
	LastBackup    time.Time
}

type statusGlobalContext struct {
	Report               StatusGlobal
	Config               ConfigFile
	TemplatesDirExpanded string
	TemplatesExists      bool
	BackupBaseExpanded   string
}

// Status calculates status for global configuration and current repository.
func Status(ctx context.Context, opts StatusOptions, deps *Dependencies, logger *slog.Logger) (StatusReport, error) {
	if logger == nil {
		panic("logger is required")
	}
	if ctx.Err() != nil {
		return StatusReport{}, ErrInterrupted
	}
	if err := validateStatusDependencies(deps); err != nil {
		return StatusReport{}, err
	}
	homeDir, err := normalizeStatusOptions(opts)
	if err != nil {
		return StatusReport{}, err
	}

	globalCtx, err := buildStatusGlobal(ctx, deps, homeDir)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{Global: globalCtx.Report}

	if opts.NoRepo {
		contractStatusPaths(&report, homeDir, deps.FileSystem.PathSeparator())
		return report, nil
	}

	repoStatus, worktrees, err := buildStatusRepo(
		ctx,
		deps,
		logger,
		globalCtx.Config,
		globalCtx.TemplatesDirExpanded,
		globalCtx.TemplatesExists,
		globalCtx.BackupBaseExpanded,
		opts.ScanBackups,
	)
	if err != nil {
		return report, err
	}
	report.Repo = repoStatus
	report.Worktrees = worktrees
	contractStatusPaths(&report, homeDir, deps.FileSystem.PathSeparator())
	return report, nil
}

func buildStatusGlobal(ctx context.Context, deps *Dependencies, homeDir string) (statusGlobalContext, error) {
	paths := buildInitPaths(deps.FileSystem, homeDir)
	configExists, err := pathExists(ctx, deps.FileSystem, paths.configPath)
	if err != nil {
		return statusGlobalContext{}, fmt.Errorf("check config path: %w", ErrCritical)
	}
	cfg, err := deps.Config.Load(ctx, paths.configPath)
	if err != nil {
		return statusGlobalContext{}, fmt.Errorf("load config: %w", ErrCritical)
	}

	templatesDir := strings.TrimSpace(DefaultTemplatesDir())
	if templatesDir == "" {
		return statusGlobalContext{}, fmt.Errorf("templates directory is empty: %w", ErrCritical)
	}
	backupBase := strings.TrimSpace(cfg.Backup.BaseDir)
	templatesDirExpanded := normalizePath(deps.FileSystem, templatesDir, homeDir)

	templatesExists, err := pathExists(ctx, deps.FileSystem, templatesDirExpanded)
	if err != nil {
		return statusGlobalContext{}, fmt.Errorf("check templates dir: %w", ErrCritical)
	}

	var backupBaseExpanded string
	var backupBaseExists bool
	if backupBase != "" {
		backupBaseExpanded = normalizePath(deps.FileSystem, backupBase, homeDir)
		backupBaseExists, err = pathExists(ctx, deps.FileSystem, backupBaseExpanded)
		if err != nil {
			return statusGlobalContext{}, fmt.Errorf("check backup base: %w", ErrCritical)
		}
	}

	logDir := strings.TrimSpace(cfg.Logging.Dir)
	var logDirExists bool
	if logDir != "" {
		logDirExpanded := normalizePath(deps.FileSystem, logDir, homeDir)
		logDirExists, err = pathExists(ctx, deps.FileSystem, logDirExpanded)
		if err != nil {
			return statusGlobalContext{}, fmt.Errorf("check log dir: %w", ErrCritical)
		}
	}

	templatesSource := "default"
	backupSource := ""
	if configExists && backupBase != "" {
		backupSource = "from config"
	}
	logDirSource := "default"
	if configExists && logDir != DefaultConfigFile().Logging.Dir {
		logDirSource = "from config"
	}

	gitTemplateDir, err := deps.Git.ConfigGetGlobal(ctx, "init.templateDir")
	if err != nil {
		return statusGlobalContext{}, fmt.Errorf("read git templateDir: %w", ErrCritical)
	}
	gitTemplateDir = strings.TrimSpace(gitTemplateDir)
	expectedTemplateDir := deps.FileSystem.Dir(templatesDir)
	expectedTemplateDirNorm := normalizePath(deps.FileSystem, expectedTemplateDir, homeDir)
	actualTemplateDirNorm := normalizePath(deps.FileSystem, gitTemplateDir, homeDir)
	gitTemplateMatches := gitTemplateDir != "" && actualTemplateDirNorm == expectedTemplateDirNorm

	report := StatusGlobal{
		ConfigFile: StatusPath{
			Path:   paths.configPath,
			Exists: configExists,
		},
		TemplatesDir: StatusPath{
			Path:   templatesDir,
			Exists: templatesExists,
			Source: templatesSource,
		},
		BackupBase: StatusPath{
			Path:   backupBase,
			Exists: backupBaseExists,
			Source: backupSource,
		},
		LogDir: StatusPath{
			Path:   logDir,
			Exists: logDirExists,
			Source: logDirSource,
		},
		GitTemplateDir: StatusGitTemplateDir{
			Expected: expectedTemplateDir,
			Actual:   gitTemplateDir,
			Set:      gitTemplateDir != "",
			Matches:  gitTemplateMatches,
			Hint:     "run: devback init",
		},
	}

	return statusGlobalContext{
		Report:               report,
		Config:               cfg,
		TemplatesDirExpanded: templatesDirExpanded,
		TemplatesExists:      templatesExists,
		BackupBaseExpanded:   backupBaseExpanded,
	}, nil
}

func buildStatusRepo(
	ctx context.Context,
	deps *Dependencies,
	logger *slog.Logger,
	cfg ConfigFile,
	templatesDirExpanded string,
	templatesExists bool,
	backupBaseExpanded string,
	scanBackupsFlag bool,
) (*StatusRepo, []WorktreeInfo, error) {
	repoRoot, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve repository root: %w", ErrCritical)
	}
	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		return nil, nil, nil
	}

	repo, err := resolveSetupRepo(ctx, deps, repoRoot)
	if err != nil {
		return nil, nil, err
	}
	repoType := RepoTypeRegular
	if repo.isWorktree {
		repoType = RepoTypeWorktree
	}

	hookFiles := statusHookFiles()
	hooksDir := repo.hooksDir
	if repo.isWorktree {
		resolved, err := resolveStatusHooksDir(ctx, deps.FileSystem, deps.Git, repo, hookFiles)
		if err != nil {
			return nil, nil, err
		}
		hooksDir = resolved
	}
	hookInstalled, hookExecutable, err := countHookFiles(ctx, deps.FileSystem, hooksDir, hookFiles)
	if err != nil {
		return nil, nil, err
	}
	hooksCurrent := StatusCurrent{Known: templatesExists}
	if templatesExists {
		matches, err := compareHooks(ctx, deps.FileSystem, hooksDir, templatesDirExpanded, hookFiles)
		if err != nil {
			return nil, nil, err
		}
		hooksCurrent.Matches = matches
	}

	backupSlug := readRepoConfig(ctx, deps.Git, repo.repoRoot, repo.isWorktree, "backup.slug")
	backupEnabled := parseBoolValue(readRepoConfig(ctx, deps.Git, repo.repoRoot, repo.isWorktree, "backup.enabled"))
	repoKey := deriveRepoKeyStatus(ctx, cfg, deps, repo.repoRoot, backupSlug, logger)

	backups := StatusBackups{}
	if scanBackupsFlag {
		backupResult, err := scanBackups(ctx, deps, backupBaseExpanded, repoKey, logger)
		if err != nil {
			return nil, nil, err
		}
		backups = backupResult
	}

	worktrees, err := deps.Git.WorktreeList(ctx, repo.repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("list worktrees: %w", ErrCritical)
	}
	currentBranch := findWorktreeBranch(deps.FileSystem, worktrees, repo.repoRoot)
	mainRoot := ""
	if repo.isWorktree {
		mainRoot = deps.FileSystem.Dir(repo.commonGit)
	}

	repoStatus := &StatusRepo{
		Root:     repo.repoRoot,
		Type:     repoType,
		Branch:   currentBranch,
		MainRoot: mainRoot,
		Hooks: StatusHooks{
			Installed:  hookInstalled,
			Executable: hookExecutable,
			Total:      len(hookFiles),
			Current:    hooksCurrent,
		},
		BackupEnabled: backupEnabled,
		BackupSlug:    backupSlug,
		RepoKey:       repoKey,
		Backups:       backups,
	}
	return repoStatus, worktrees, nil
}

func resolveStatusHooksDir(
	ctx context.Context,
	fs FileSystemPort,
	git GitPort,
	repo setupRepo,
	hookFiles []string,
) (string, error) {
	if !repo.isWorktree {
		return repo.hooksDir, nil
	}
	installed, err := hooksInstalled(ctx, fs, repo.hooksDir, hookFiles)
	if err != nil {
		return repo.hooksDir, err
	}
	if installed {
		return repo.hooksDir, nil
	}
	if git == nil {
		return repo.hooksDir, nil
	}
	gitDir, err := git.GitDir(ctx, repo.repoRoot)
	if err != nil {
		return repo.hooksDir, nil
	}
	gitDirPath := strings.TrimSpace(gitDir)
	if !isAbsPath(gitDirPath) {
		gitDirPath = fs.Join(repo.repoRoot, gitDirPath)
	}
	if !looksLikeWorktreeGitDir(fs, gitDirPath) {
		return repo.hooksDir, nil
	}
	commonDir := fs.Dir(fs.Dir(gitDirPath))
	if normalizeRepoPath(fs, commonDir) == normalizeRepoPath(fs, repo.commonGit) {
		return repo.hooksDir, nil
	}
	derivedHooks := fs.Join(commonDir, "hooks")
	installed, err = hooksInstalled(ctx, fs, derivedHooks, hookFiles)
	if err != nil {
		return repo.hooksDir, err
	}
	if installed {
		return derivedHooks, nil
	}
	return repo.hooksDir, nil
}

func looksLikeWorktreeGitDir(fs FileSystemPort, path string) bool {
	clean := normalizeRepoPath(fs, path)
	if strings.Contains(clean, "/worktrees/") {
		return true
	}
	return strings.Contains(clean, "\\worktrees\\")
}

func findWorktreeBranch(fs FileSystemPort, worktrees []WorktreeInfo, repoRoot string) string {
	root := normalizeRepoPath(fs, repoRoot)
	for _, wt := range worktrees {
		if normalizeRepoPath(fs, wt.Path) == root {
			return strings.TrimSpace(wt.Branch)
		}
	}
	return ""
}

// FormatStatus renders the status report into human-readable output.
func FormatStatus(report StatusReport, useColor bool) string {
	p := newStatusPalette(useColor)
	var b strings.Builder

	fmt.Fprintf(&b, "%sDevBack Status%s\n", p.bold, p.reset)
	b.WriteString(strings.Repeat("─", 54))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%sGlobal Configuration:%s\n", p.boldCyan, p.reset)
	appendStatusLine(&b, "Config file:", formatPathStatus(report.Global.ConfigFile, p))
	appendStatusLine(&b, "Hook templates:", formatPathStatus(report.Global.TemplatesDir, p))
	appendStatusLine(&b, "Backup base:", formatPathStatus(report.Global.BackupBase, p))
	appendStatusLine(&b, "Log dir:", formatPathStatus(report.Global.LogDir, p))
	appendStatusLine(&b, "Git templateDir:", formatGitTemplateDir(report.Global.GitTemplateDir, p))

	if report.Repo == nil {
		return b.String()
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "%sCurrent Repository:%s %s\n", p.boldCyan, p.reset, report.Repo.Root)
	appendStatusLine(&b, "Type:", report.Repo.Type.String())
	appendStatusLine(&b, "Branch:", formatTextValue(report.Repo.Branch, p))
	if report.Repo.Type == RepoTypeWorktree {
		appendStatusLine(&b, "Main repository:", formatTextValue(report.Repo.MainRoot, p))
	}
	appendStatusLine(&b, "Hooks installed:", formatCountStatus(report.Repo.Hooks.Installed, report.Repo.Hooks.Total, p))
	appendStatusLine(&b, "Hooks executable:", formatCountStatus(report.Repo.Hooks.Executable, report.Repo.Hooks.Total, p))
	appendStatusLine(&b, "Hooks current:", formatHooksCurrent(report.Repo.Hooks.Current, p))
	appendStatusLine(&b, "Backup enabled:", formatBoolStatus(report.Repo.BackupEnabled, p))
	appendStatusLine(&b, "Backup slug:", formatTextValue(report.Repo.BackupSlug, p))
	appendStatusLine(&b, "Repo key:", formatTextValue(report.Repo.RepoKey, p))

	if report.Repo.Backups.Scanned {
		appendStatusLine(&b, "Last backup:", formatBackupTime(report.Repo.Backups.LastBackup, p))
		appendStatusLine(&b, "Snapshots:", fmt.Sprintf("%d", report.Repo.Backups.SnapshotCount))
		appendStatusLine(&b, "Size:", formatBackupSize(report.Repo.Backups.TotalSizeKB))
	} else {
		appendStatusLine(&b, "Last backup:", fmt.Sprintf("%s(use --scan-backups)%s", p.dim, p.reset))
		appendStatusLine(&b, "Snapshots:", fmt.Sprintf("%s(use --scan-backups)%s", p.dim, p.reset))
		appendStatusLine(&b, "Size:", fmt.Sprintf("%s(use --scan-backups)%s", p.dim, p.reset))
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "%sWorktrees:%s\n", p.boldCyan, p.reset)
	if len(report.Worktrees) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}
	for _, wt := range report.Worktrees {
		isCurrent := report.Repo != nil &&
			normalizeComparableStatusPath(wt.Path) == normalizeComparableStatusPath(report.Repo.Root)
		switch {
		case isCurrent:
			fmt.Fprintf(&b, "  %s▶ %s  [%s]%s\n", p.green, wt.Path, wt.Branch, p.reset)
		case wt.Branch == "":
			fmt.Fprintf(&b, "    %s\n", wt.Path)
		default:
			fmt.Fprintf(&b, "    %s  %s[%s]%s\n", wt.Path, p.cyan, wt.Branch, p.reset)
		}
	}
	return b.String()
}

func normalizeComparableStatusPath(p string) string {
	clean := strings.TrimSpace(p)
	if clean == "" {
		return ""
	}
	// Use slash-based normalization to compare equivalent paths from different sources.
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = path.Clean(clean)
	return strings.TrimRight(clean, "/")
}

func contractStatusPaths(report *StatusReport, homeDir string, sep byte) {
	report.Global.ConfigFile.Path = contractHomeDir(report.Global.ConfigFile.Path, homeDir, sep)
	report.Global.TemplatesDir.Path = contractHomeDir(report.Global.TemplatesDir.Path, homeDir, sep)
	report.Global.BackupBase.Path = contractHomeDir(report.Global.BackupBase.Path, homeDir, sep)
	report.Global.LogDir.Path = contractHomeDir(report.Global.LogDir.Path, homeDir, sep)
	report.Global.GitTemplateDir.Expected = contractHomeDir(report.Global.GitTemplateDir.Expected, homeDir, sep)
	report.Global.GitTemplateDir.Actual = contractHomeDir(report.Global.GitTemplateDir.Actual, homeDir, sep)
	if report.Repo != nil {
		report.Repo.Root = contractHomeDir(report.Repo.Root, homeDir, sep)
		report.Repo.MainRoot = contractHomeDir(report.Repo.MainRoot, homeDir, sep)
	}
	for i := range report.Worktrees {
		report.Worktrees[i].Path = contractHomeDir(report.Worktrees[i].Path, homeDir, sep)
	}
}

func validateStatusDependencies(deps *Dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are required: %w", ErrCritical)
	}
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available: %w", ErrCritical)
	}
	if deps.Config == nil {
		return fmt.Errorf("config adapter not available: %w", ErrCritical)
	}
	if deps.Git == nil {
		return fmt.Errorf("git adapter not available: %w", ErrCritical)
	}
	return nil
}

func normalizeStatusOptions(opts StatusOptions) (string, error) {
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		return "", fmt.Errorf("home directory is empty: %w", ErrCritical)
	}
	return homeDir, nil
}

func statusHookFiles() []string {
	return []string{
		"post-commit",
		"post-merge",
		"post-rewrite",
	}
}

func countHookFiles(ctx context.Context, fs FileSystemPort, hooksDir string, names []string) (int, int, error) {
	var installed int
	var executable int
	for _, name := range names {
		if ctx.Err() != nil {
			return 0, 0, ErrInterrupted
		}
		target := fs.Join(hooksDir, name)
		exists, err := pathExists(ctx, fs, target)
		if err != nil {
			return 0, 0, fmt.Errorf("check hook %s: %w", name, ErrCritical)
		}
		if !exists {
			continue
		}
		installed++
		info, err := fs.Stat(ctx, target)
		if err != nil {
			return 0, 0, fmt.Errorf("stat hook %s: %w", name, ErrCritical)
		}
		if info != nil && info.Mode()&0o111 != 0 {
			executable++
		}
	}
	return installed, executable, nil
}

func compareHooks(
	ctx context.Context,
	fs FileSystemPort,
	hooksDir string,
	templatesDir string,
	names []string,
) (bool, error) {
	for _, name := range names {
		if ctx.Err() != nil {
			return false, ErrInterrupted
		}
		hookPath := fs.Join(hooksDir, name)
		templatePath := fs.Join(templatesDir, name)
		hookExists, err := pathExists(ctx, fs, hookPath)
		if err != nil {
			return false, fmt.Errorf("check hook %s: %w", name, ErrCritical)
		}
		templateExists, err := pathExists(ctx, fs, templatePath)
		if err != nil {
			return false, fmt.Errorf("check template %s: %w", name, ErrCritical)
		}
		if !hookExists || !templateExists {
			return false, nil
		}
		hookData, err := fs.ReadFile(ctx, hookPath)
		if err != nil {
			return false, fmt.Errorf("read hook %s: %w", name, ErrCritical)
		}
		templateData, err := fs.ReadFile(ctx, templatePath)
		if err != nil {
			return false, fmt.Errorf("read template %s: %w", name, ErrCritical)
		}
		if !bytes.Equal(normalizeLineEndings(hookData), normalizeLineEndings(templateData)) {
			return false, nil
		}
	}
	return true, nil
}

func normalizeLineEndings(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r"), nil)
}

func readRepoConfig(ctx context.Context, git GitPort, repoRoot string, preferWorktree bool, key string) string {
	if preferWorktree {
		if value, err := git.ConfigGetWorktree(ctx, repoRoot, key); err == nil {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	if value, err := git.ConfigGet(ctx, repoRoot, key); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func parseBoolValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func deriveRepoKeyStatus(
	ctx context.Context,
	cfg ConfigFile,
	deps *Dependencies,
	repoRoot string,
	backupSlug string,
	logger *slog.Logger,
) string {
	style := strings.TrimSpace(cfg.RepoKey.Style)
	if style == "" {
		style = repoKeyStyleAuto
	}
	compat := &Config{
		AutoRemoteMerge: cfg.RepoKey.AutoRemoteMerge,
		RemoteHashLen:   cfg.RepoKey.RemoteHashLen,
	}

	switch style {
	case repoKeyStyleAuto:
		if backupSlug != "" {
			if key, ok := repoKeyFromSlug(deps.FileSystem, repoRoot, backupSlug); ok {
				return key
			}
		}
		if remote, err := deps.Git.ConfigGet(ctx, repoRoot, "remote.origin.url"); err == nil && remote != "" {
			if key, ok := repoKeyFromRemote(deps.FileSystem, remote, repoRoot, compat); ok {
				return key
			}
		}
	case repoKeyStyleCustom:
		if backupSlug != "" {
			if key, ok := repoKeyFromSlug(deps.FileSystem, repoRoot, backupSlug); ok {
				return key
			}
		}
	case repoKeyStyleRemoteHierarchy:
		bc := newBackupContext(logger, false)
		if key, ok := deriveRepoKeyRemoteHierarchy(ctx, deps, repoRoot, bc); ok {
			return key
		}
	}

	return repoKeyNameHash(deps.FileSystem, repoRoot)
}

func scanBackups(
	ctx context.Context,
	deps *Dependencies,
	backupBase string,
	repoKey string,
	logger *slog.Logger,
) (StatusBackups, error) {
	result := StatusBackups{Scanned: true}
	if backupBase == "" || repoKey == "" {
		return result, nil
	}
	repoDir := deps.FileSystem.Join(backupBase, repoKey)
	exists, err := pathExists(ctx, deps.FileSystem, repoDir)
	if err != nil {
		return result, fmt.Errorf("check backups dir: %w", ErrCritical)
	}
	if !exists {
		return result, nil
	}
	snaps, err := listSnapshots(ctx, deps, repoDir)
	if err != nil {
		return result, fmt.Errorf("list snapshots: %w", ErrCritical)
	}
	result.SnapshotCount = len(snaps)
	if len(snaps) == 0 {
		return result, nil
	}
	bc := newBackupContext(logger, false)
	var latest time.Time
	for _, snap := range snaps {
		if ctx.Err() != nil {
			return result, ErrInterrupted
		}
		kb, err := dirSizeKB(ctx, deps, snap.TimeDir, bc)
		if err != nil {
			return result, fmt.Errorf("scan snapshot size: %w", ErrCritical)
		}
		result.TotalSizeKB += kb
		info, err := deps.FileSystem.Stat(ctx, snap.TimeDir)
		if err != nil {
			return result, fmt.Errorf("stat snapshot: %w", ErrCritical)
		}
		if info != nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	result.LastBackup = latest
	return result, nil
}

func appendStatusLine(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-18s %s\n", label, value)
}

func formatPathStatus(path StatusPath, p statusPalette) string {
	if path.Path == "" {
		return fmt.Sprintf("%s✗%s %s%s%s", p.red, p.reset, p.dim, statusNotSet, p.reset)
	}
	var status string
	if path.Exists {
		status = fmt.Sprintf("%s✓%s", p.green, p.reset)
	} else {
		status = fmt.Sprintf("%s✗%s %s(not found)%s", p.red, p.reset, p.dim, p.reset)
	}
	value := fmt.Sprintf("%s %s", path.Path, status)
	if path.Source != "" {
		value += fmt.Sprintf(" %s(%s)%s", p.dim, path.Source, p.reset)
	}
	return value
}

func formatGitTemplateDir(status StatusGitTemplateDir, p statusPalette) string {
	if !status.Set {
		if status.Hint != "" {
			return fmt.Sprintf("%s–%s %s(not set, %s%s%s)%s",
				p.yellow, p.reset, p.dim,
				p.reset+p.yellow, status.Hint, p.reset+p.dim, p.reset)
		}
		return fmt.Sprintf("%s–%s %s%s%s", p.yellow, p.reset, p.dim, statusNotSet, p.reset)
	}
	if status.Matches {
		return fmt.Sprintf("%s %s✓%s", status.Expected, p.green, p.reset)
	}
	return fmt.Sprintf("%s %s(differs)%s", status.Actual, p.dim, p.reset)
}

func formatCountStatus(count, total int, p statusPalette) string {
	if total == 0 {
		return fmt.Sprintf("%s(unknown)%s", p.dim, p.reset)
	}
	return fmt.Sprintf("%s (%d of %d)", formatBoolStatus(count == total, p), count, total)
}

func formatHooksCurrent(status StatusCurrent, p statusPalette) string {
	if !status.Known {
		return fmt.Sprintf("%s(unknown)%s", p.dim, p.reset)
	}
	if status.Matches {
		return fmt.Sprintf("%s✓%s (matches global templates)", p.green, p.reset)
	}
	return fmt.Sprintf("%s✗%s (%s%s%s)", p.red, p.reset, p.yellow, "run: devback setup --force", p.reset)
}

func formatBoolStatus(value bool, p statusPalette) string {
	if value {
		return fmt.Sprintf("%s✓%s", p.green, p.reset)
	}
	return fmt.Sprintf("%s✗%s", p.red, p.reset)
}

func formatTextValue(value string, p statusPalette) string {
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf("%s–%s %s%s%s", p.yellow, p.reset, p.dim, statusNotSet, p.reset)
	}
	return value
}

func formatBackupTime(value time.Time, p statusPalette) string {
	if value.IsZero() {
		return fmt.Sprintf("%s✗%s %s(not found)%s", p.red, p.reset, p.dim, p.reset)
	}
	return value.Format("2006-01-02 15:04:05")
}

func formatBackupSize(kb int64) string {
	return humanKB(kb)
}
