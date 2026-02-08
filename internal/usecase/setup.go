package usecase

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// SetupOptions describes repository setup behavior.
type SetupOptions struct {
	Slug    string
	Force   bool
	NoHooks bool
	DryRun  bool
	HomeDir string
}

type setupInputs struct {
	homeDir string
	slug    string
}

type setupRepo struct {
	repoRoot   string
	gitDir     string
	commonGit  string
	hooksDir   string
	isWorktree bool
}

// RepoType describes repository layout.
type RepoType int

const (
	// RepoTypeRegular represents a standard repository.
	RepoTypeRegular RepoType = iota
	// RepoTypeWorktree represents a linked worktree.
	RepoTypeWorktree
)

func (r RepoType) String() string {
	switch r {
	case RepoTypeWorktree:
		return "Worktree"
	default:
		return "Regular repository"
	}
}

// Setup configures current repository for DevBack.
func Setup(ctx context.Context, opts SetupOptions, deps *Dependencies, logger *slog.Logger) error {
	if logger == nil {
		panic("logger is required")
	}
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if err := validateSetupDependencies(deps); err != nil {
		return err
	}
	inputs, err := normalizeSetupOptions(opts)
	if err != nil {
		return err
	}

	repoRoot, err := resolveRepoRoot(ctx, deps)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", ErrCritical)
	}
	if err := ensureGitRepo(ctx, deps, repoRoot); err != nil {
		return fmt.Errorf("not a git repository: %w", ErrUsage)
	}

	repo, err := resolveSetupRepo(ctx, deps, repoRoot)
	if err != nil {
		return err
	}
	if repo.isWorktree && opts.Force {
		return fmt.Errorf("--force is not supported in worktree; run setup in main repository: %w", ErrUsage)
	}

	templatesDir, err := normalizeTemplatesDir(DefaultTemplatesDir(), inputs.homeDir)
	if err != nil {
		return err
	}
	templateFiles, err := listTemplateFiles(ctx, deps.FileSystem, templatesDir)
	if err != nil {
		return err
	}
	requiredFiles := setupRequiredFiles()

	if err := applySetupHooks(ctx, deps, repo, templatesDir, templateFiles, requiredFiles, opts, logger); err != nil {
		return err
	}
	if err := installDevbackIgnore(ctx, deps, repo, inputs.homeDir, opts.DryRun, logger); err != nil {
		return err
	}
	if err := applySetupSlug(ctx, deps, repo, inputs.slug, opts.DryRun); err != nil {
		return err
	}

	logger.InfoContext(ctx, "Setup completed")
	return nil
}

func validateSetupDependencies(deps *Dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are required: %w", ErrCritical)
	}
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available: %w", ErrCritical)
	}
	if deps.Git == nil {
		return fmt.Errorf("git adapter not available: %w", ErrCritical)
	}
	return nil
}

func normalizeSetupOptions(opts SetupOptions) (setupInputs, error) {
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		return setupInputs{}, fmt.Errorf("home directory is empty: %w", ErrCritical)
	}
	slug := strings.TrimSpace(opts.Slug)
	if opts.Slug != "" && slug == "" {
		return setupInputs{}, fmt.Errorf("slug is empty: %w", ErrUsage)
	}
	return setupInputs{
		homeDir: homeDir,
		slug:    slug,
	}, nil
}

func resolveSetupRepo(ctx context.Context, deps *Dependencies, repoRoot string) (setupRepo, error) {
	gitDir, err := deps.Git.GitDir(ctx, repoRoot)
	if err != nil {
		return setupRepo{}, fmt.Errorf("resolve git dir: %w", ErrCritical)
	}
	commonGit, err := deps.Git.GitCommonDir(ctx, repoRoot)
	if err != nil {
		return setupRepo{}, fmt.Errorf("resolve git common dir: %w", ErrCritical)
	}
	gitDir = strings.TrimSpace(gitDir)
	commonGit = strings.TrimSpace(commonGit)
	if gitDir == "" {
		return setupRepo{}, fmt.Errorf("git dir is empty: %w", ErrCritical)
	}
	if commonGit == "" {
		return setupRepo{}, fmt.Errorf("git common dir is empty: %w", ErrCritical)
	}

	gitDirPath := gitDir
	if !isAbsPath(gitDirPath) {
		gitDirPath = deps.FileSystem.Join(repoRoot, gitDir)
	}
	commonDirPath := commonGit
	if !isAbsPath(commonDirPath) {
		commonDirPath = deps.FileSystem.Join(repoRoot, commonGit)
	}
	isWorktree := normalizeRepoPath(deps.FileSystem, gitDirPath) != normalizeRepoPath(deps.FileSystem, commonDirPath)
	hooksDir := deps.FileSystem.Join(gitDirPath, "hooks")
	if isWorktree {
		hooksDir = deps.FileSystem.Join(commonDirPath, "hooks")
	}

	return setupRepo{
		repoRoot:   repoRoot,
		gitDir:     gitDirPath,
		commonGit:  commonDirPath,
		hooksDir:   hooksDir,
		isWorktree: isWorktree,
	}, nil
}

func listTemplateFiles(ctx context.Context, fs FileSystemPort, templatesDir string) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ErrInterrupted
	}
	exists, err := pathExists(ctx, fs, templatesDir)
	if err != nil {
		return nil, fmt.Errorf("check templates dir: %w", ErrCritical)
	}
	if !exists {
		return nil, fmt.Errorf("templates directory %s not found; run devback init: %w", templatesDir, ErrUsage)
	}

	entries, err := fs.ReadDir(ctx, templatesDir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", ErrCritical)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("templates directory %s is empty: %w", templatesDir, ErrCritical)
	}
	sort.Strings(files)
	return files, nil
}

func applySetupHooks(
	ctx context.Context,
	deps *Dependencies,
	repo setupRepo,
	templatesDir string,
	templateFiles []string,
	requiredFiles []string,
	opts SetupOptions,
	logger *slog.Logger,
) error {
	if opts.NoHooks {
		if repo.isWorktree {
			ok, err := hooksInstalled(ctx, deps.FileSystem, repo.hooksDir, requiredFiles)
			if err != nil {
				return err
			}
			if !ok {
				logger.WarnContext(
					ctx,
					"Hooks are not configured in main repository; run setup in main to enable backups",
				)
			}
		}
		return nil
	}

	if repo.isWorktree {
		ok, err := hooksInstalled(ctx, deps.FileSystem, repo.hooksDir, requiredFiles)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf(
				"hooks are not configured in main repository; run devback setup in main: %w",
				ErrUsage,
			)
		}
		return setBackupEnabled(ctx, deps.Git, repo.repoRoot, opts.DryRun)
	}

	if opts.Force {
		if err := installHooks(ctx, deps, templatesDir, repo.hooksDir, templateFiles, opts.DryRun); err != nil {
			return err
		}
	} else {
		if err := installHooksMerged(ctx, deps, templatesDir, repo.hooksDir, templateFiles, opts.DryRun, logger); err != nil {
			return err
		}
	}
	return setBackupEnabled(ctx, deps.Git, repo.repoRoot, opts.DryRun)
}

func setupRequiredFiles() []string {
	return append([]string{}, statusHookFiles()...)
}

func applySetupSlug(
	ctx context.Context,
	deps *Dependencies,
	repo setupRepo,
	slug string,
	dryRun bool,
) error {
	if slug == "" {
		return nil
	}
	if repo.isWorktree {
		if err := enableWorktreeConfig(ctx, deps.Git, repo.repoRoot, dryRun); err != nil {
			return err
		}
		if dryRun {
			return nil
		}
		if err := deps.Git.ConfigSetWorktree(ctx, repo.repoRoot, "backup.slug", slug); err != nil {
			return fmt.Errorf("set git config backup.slug (worktree): %w", ErrCritical)
		}
		return nil
	}

	if dryRun {
		return nil
	}
	if err := deps.Git.ConfigSet(ctx, repo.repoRoot, "backup.slug", slug); err != nil {
		return fmt.Errorf("set git config backup.slug: %w", ErrCritical)
	}
	return nil
}

const devbackIgnoreFile = ".devbackignore"

func installDevbackIgnore(
	ctx context.Context,
	deps *Dependencies,
	repo setupRepo,
	homeDir string,
	dryRun bool,
	logger *slog.Logger,
) error {
	if repo.isWorktree {
		return nil
	}

	repoTemplatesDir, err := normalizeTemplatesDir(DefaultRepoTemplatesDir(), homeDir)
	if err != nil {
		return err
	}

	srcPath := deps.FileSystem.Join(repoTemplatesDir, "devbackignore")
	srcExists, err := pathExists(ctx, deps.FileSystem, srcPath)
	if err != nil {
		return fmt.Errorf("check repo template devbackignore: %w", ErrCritical)
	}
	if !srcExists {
		logger.WarnContext(ctx, "Repo template devbackignore not found, skipping", "path", srcPath)
		return nil
	}
	srcInfo, err := deps.FileSystem.Stat(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("stat repo template devbackignore: %w", ErrCritical)
	}

	dstPath := deps.FileSystem.Join(repo.repoRoot, devbackIgnoreFile)
	dstExists, err := pathExists(ctx, deps.FileSystem, dstPath)
	if err != nil {
		return fmt.Errorf("check %s: %w", devbackIgnoreFile, ErrCritical)
	}
	if dstExists {
		logger.InfoContext(ctx, ".devbackignore already exists, skipping", "path", dstPath)
		return nil
	}

	if dryRun {
		return nil
	}

	data, err := deps.FileSystem.ReadFile(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("read repo template devbackignore: %w", ErrCritical)
	}
	if err := deps.FileSystem.WriteFile(ctx, dstPath, data, srcInfo.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", devbackIgnoreFile, ErrCritical)
	}

	logger.InfoContext(ctx, "Created .devbackignore", "path", dstPath)
	return nil
}

func hooksInstalled(ctx context.Context, fs FileSystemPort, hooksDir string, files []string) (bool, error) {
	for _, name := range files {
		if ctx.Err() != nil {
			return false, ErrInterrupted
		}
		target := fs.Join(hooksDir, name)
		exists, err := pathExists(ctx, fs, target)
		if err != nil {
			return false, fmt.Errorf("check hook %s: %w", name, ErrCritical)
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

const devbackMergedHookMarker = "devback-merged-hook"

func installHooksMerged(
	ctx context.Context,
	deps *Dependencies,
	templatesDir string,
	hooksDir string,
	files []string,
	dryRun bool,
	logger *slog.Logger,
) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if dryRun {
		return nil
	}
	if err := deps.FileSystem.CreateDir(ctx, hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", ErrCritical)
	}
	for _, name := range files {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		if err := installMergedHook(ctx, deps, templatesDir, hooksDir, name, logger); err != nil {
			return err
		}
	}
	return nil
}

func installMergedHook(
	ctx context.Context,
	deps *Dependencies,
	templatesDir string,
	hooksDir string,
	name string,
	logger *slog.Logger,
) error {
	dst := deps.FileSystem.Join(hooksDir, name)
	exists, err := pathExists(ctx, deps.FileSystem, dst)
	if err != nil {
		return fmt.Errorf("check hook %s: %w", name, ErrCritical)
	}
	if !exists {
		return copyHookTemplate(ctx, deps, templatesDir, hooksDir, name)
	}
	if !isExecutableSetupFile(name) {
		return nil
	}
	data, err := deps.FileSystem.ReadFile(ctx, dst)
	if err != nil {
		return fmt.Errorf("read hook %s: %w", name, ErrCritical)
	}
	if isDevbackHookContent(data, name) {
		if err := deps.FileSystem.Chmod(ctx, dst, setupFileMode(name)); err != nil {
			return fmt.Errorf("chmod hook %s: %w", name, ErrCritical)
		}
		return nil
	}
	devbackPath, err := devbackPathFromTemplate(ctx, deps.FileSystem, templatesDir, name)
	if err != nil {
		return err
	}
	backupName, backupPath, err := reserveHookBackup(ctx, deps.FileSystem, hooksDir, name)
	if err != nil {
		return err
	}
	if err := deps.FileSystem.Move(ctx, dst, backupPath); err != nil {
		return fmt.Errorf("backup hook %s: %w", name, ErrCritical)
	}
	mergedHook := buildMergedHook(name, devbackPath, backupName)
	if err := deps.FileSystem.WriteFile(ctx, dst, mergedHook, 0o755); err != nil {
		return fmt.Errorf("write merged hook %s: %w", name, ErrCritical)
	}
	if err := deps.FileSystem.Chmod(ctx, dst, setupFileMode(name)); err != nil {
		return fmt.Errorf("chmod hook %s: %w", name, ErrCritical)
	}
	if logger != nil {
		logger.InfoContext(ctx, "Merged existing hook", "hook", name, "backup", backupName)
	}
	return nil
}

func devbackPathFromTemplate(
	ctx context.Context,
	fs FileSystemPort,
	templatesDir string,
	name string,
) (string, error) {
	templatePath := fs.Join(templatesDir, name)
	templateData, err := fs.ReadFile(ctx, templatePath)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", name, ErrCritical)
	}
	devbackPath, err := extractDevbackPath(templateData)
	if err != nil {
		return "", fmt.Errorf("parse hook template %s: %w", name, ErrCritical)
	}
	return devbackPath, nil
}

func copyHookTemplate(
	ctx context.Context,
	deps *Dependencies,
	templatesDir string,
	hooksDir string,
	name string,
) error {
	src := deps.FileSystem.Join(templatesDir, name)
	dst := deps.FileSystem.Join(hooksDir, name)
	if err := deps.FileSystem.Copy(ctx, src, dst); err != nil {
		return fmt.Errorf("copy hook %s: %w", name, ErrCritical)
	}
	mode := setupFileMode(name)
	if err := deps.FileSystem.Chmod(ctx, dst, mode); err != nil {
		return fmt.Errorf("chmod hook %s: %w", name, ErrCritical)
	}
	return nil
}

func reserveHookBackup(
	ctx context.Context,
	fs FileSystemPort,
	hooksDir string,
	name string,
) (string, string, error) {
	base := name + ".devback.orig"
	candidate := fs.Join(hooksDir, base)
	exists, err := pathExists(ctx, fs, candidate)
	if err != nil {
		return "", "", fmt.Errorf("check hook backup %s: %w", name, ErrCritical)
	}
	if !exists {
		return base, candidate, nil
	}
	for i := 1; i <= 20; i++ {
		alt := fmt.Sprintf("%s.%d", base, i)
		altPath := fs.Join(hooksDir, alt)
		exists, err := pathExists(ctx, fs, altPath)
		if err != nil {
			return "", "", fmt.Errorf("check hook backup %s: %w", name, ErrCritical)
		}
		if !exists {
			return alt, altPath, nil
		}
	}
	return "", "", fmt.Errorf("cannot allocate hook backup for %s: %w", name, ErrCritical)
}

func isDevbackHookContent(data []byte, hookName string) bool {
	clean := normalizeLineEndings(data)
	if bytes.Contains(clean, []byte(devbackMergedHookMarker)) {
		return true
	}
	if bytes.Contains(clean, []byte("devback hook "+hookName)) {
		return true
	}
	if bytes.Contains(clean, []byte("DEVBACK=")) && bytes.Contains(clean, []byte("hook "+hookName)) {
		return true
	}
	return false
}

func extractDevbackPath(template []byte) (string, error) {
	lines := strings.Split(string(normalizeLineEndings(template)), "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if !strings.HasPrefix(clean, "DEVBACK=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(clean, "DEVBACK="))
		value = strings.Trim(value, "\"'")
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("DEVBACK path not found")
}

func buildMergedHook(hookName, devbackPath, backupName string) []byte {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# " + devbackMergedHookMarker + ": " + hookName + "\n")
	b.WriteString("HOOK_DIR=$(CDPATH= cd \"$(dirname \"$0\")\" && pwd)\n")
	b.WriteString("ORIG_HOOK=\"$HOOK_DIR/" + backupName + "\"\n")
	b.WriteString("orig_status=0\n")
	b.WriteString("if [ -x \"$ORIG_HOOK\" ]; then\n")
	b.WriteString("  \"$ORIG_HOOK\" \"$@\"\n")
	b.WriteString("  orig_status=$?\n")
	b.WriteString("fi\n")
	b.WriteString("DEVBACK=\"" + devbackPath + "\"\n")
	b.WriteString("[ -x \"$DEVBACK\" ] || { echo \"SKIP: devback not found at $DEVBACK\" >&2; exit $orig_status; }\n")
	b.WriteString("\"$DEVBACK\" hook " + hookName + " \"$@\"\n")
	b.WriteString("devback_status=$?\n")
	b.WriteString("if [ $orig_status -ne 0 ]; then\n")
	b.WriteString("  exit $orig_status\n")
	b.WriteString("fi\n")
	b.WriteString("exit $devback_status\n")
	return []byte(b.String())
}

func installHooks(
	ctx context.Context,
	deps *Dependencies,
	templatesDir string,
	hooksDir string,
	files []string,
	dryRun bool,
) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if dryRun {
		return nil
	}
	if err := deps.FileSystem.CreateDir(ctx, hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", ErrCritical)
	}
	for _, name := range files {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		if err := copyHookTemplate(ctx, deps, templatesDir, hooksDir, name); err != nil {
			return err
		}
	}
	return nil
}

func setBackupEnabled(ctx context.Context, git GitPort, repoRoot string, dryRun bool) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if dryRun {
		return nil
	}
	if err := git.ConfigSet(ctx, repoRoot, "backup.enabled", "true"); err != nil {
		return fmt.Errorf("set git config backup.enabled: %w", ErrCritical)
	}
	return nil
}

func enableWorktreeConfig(ctx context.Context, git GitPort, repoRoot string, dryRun bool) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	if dryRun {
		return nil
	}
	if err := git.ConfigSet(ctx, repoRoot, "extensions.worktreeConfig", "true"); err != nil {
		return fmt.Errorf("enable worktree config: %w", ErrUsage)
	}
	return nil
}

func normalizeRepoPath(fs FileSystemPort, path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	cleaned := fs.Clean(clean)
	if cleaned == "." {
		return ""
	}
	return trimTrailingSeparators(fs, cleaned)
}

func isAbsPath(path string) bool {
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return true
	}
	return len(path) >= 2 && path[1] == ':'
}

func setupFileMode(name string) int {
	if isExecutableSetupFile(name) {
		return 0o755
	}
	return 0o644
}

func isExecutableSetupFile(name string) bool {
	for _, hook := range statusHookFiles() {
		if name == hook {
			return true
		}
	}
	return false
}
