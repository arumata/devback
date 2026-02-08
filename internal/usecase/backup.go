package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type backupContext struct {
	logger  *slog.Logger
	verbose bool
}

func newBackupContext(logger *slog.Logger, verbose bool) *backupContext {
	if logger == nil {
		panic("logger is required")
	}
	return &backupContext{logger: logger, verbose: verbose}
}

func (bc *backupContext) logf(format string, a ...any) {
	bc.logger.Info(fmt.Sprintf(format, a...))
}

func (bc *backupContext) vlogf(format string, a ...any) {
	if !bc.verbose {
		return
	}
	bc.logf(format, a...)
}

func (bc *backupContext) warnf(format string, a ...any) {
	bc.logger.Warn(fmt.Sprintf(format, a...))
}

func resolveRepoRoot(ctx context.Context, deps *Dependencies) (string, error) {
	if deps.Git != nil {
		if root, err := deps.Git.RepoRoot(ctx); err == nil {
			return root, nil
		}
	}
	if deps.FileSystem == nil {
		return "", fmt.Errorf("filesystem adapter not available")
	}
	return deps.FileSystem.GetWorkingDir(ctx)
}

func ensureGitRepo(ctx context.Context, deps *Dependencies, repoRoot string) error {
	if deps.Git != nil {
		return ensureGitRepoWithGitAdapter(ctx, deps, repoRoot)
	}
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available")
	}
	return ensureGitRepoFromDotGit(ctx, deps, repoRoot)
}

func ensureGitRepoWithGitAdapter(ctx context.Context, deps *Dependencies, repoRoot string) error {
	gitDir, err := deps.Git.GitDir(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("not a git repository in %s: %w", repoRoot, err)
	}
	if strings.TrimSpace(gitDir) == "" {
		return fmt.Errorf("git dir is empty in %s", repoRoot)
	}
	return validateGitDirPath(ctx, deps, repoRoot, gitDir, "git dir")
}

func ensureGitRepoFromDotGit(ctx context.Context, deps *Dependencies, repoRoot string) error {
	gitDir := deps.FileSystem.Join(repoRoot, ".git")
	info, err := deps.FileSystem.Stat(ctx, gitDir)
	if err != nil {
		return fmt.Errorf(".git not found in %s: %w", repoRoot, err)
	}
	if info == nil || info.IsDir() {
		return nil
	}
	if !info.IsRegular() {
		return fmt.Errorf(".git is not a regular file or directory in %s", repoRoot)
	}
	data, err := deps.FileSystem.ReadFile(ctx, gitDir)
	if err != nil {
		return fmt.Errorf("read .git file in %s: %w", repoRoot, err)
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if !strings.HasPrefix(line, "gitdir:") {
		return fmt.Errorf("unsupported .git file format in %s", repoRoot)
	}
	gitDirPath := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if gitDirPath == "" {
		return fmt.Errorf("empty gitdir in .git file for %s", repoRoot)
	}
	return validateGitDirPath(ctx, deps, repoRoot, gitDirPath, "gitdir")
}

func validateGitDirPath(ctx context.Context, deps *Dependencies, repoRoot, gitDirPath, label string) error {
	if deps.FileSystem == nil {
		return fmt.Errorf("filesystem adapter not available")
	}
	if strings.TrimSpace(gitDirPath) == "" {
		return fmt.Errorf("%s is empty in %s", label, repoRoot)
	}
	if !deps.FileSystem.IsAbs(gitDirPath) {
		gitDirPath = deps.FileSystem.Join(repoRoot, gitDirPath)
	}
	info, err := deps.FileSystem.Stat(ctx, gitDirPath)
	if err != nil {
		return fmt.Errorf("%s not found in %s: %w", label, repoRoot, err)
	}
	if info != nil && !info.IsDir() {
		return fmt.Errorf("%s is not a directory in %s", label, repoRoot)
	}
	return nil
}

type snapshotGitDirs struct {
	gitDir     string
	commonDir  string
	isWorktree bool
}

func resolveSnapshotGitDirs(ctx context.Context, deps *Dependencies, repoRoot string) (snapshotGitDirs, error) {
	if deps.Git == nil {
		return snapshotGitDirs{}, fmt.Errorf("git adapter not available")
	}
	if deps.FileSystem == nil {
		return snapshotGitDirs{}, fmt.Errorf("filesystem adapter not available")
	}
	gitDir, err := deps.Git.GitDir(ctx, repoRoot)
	if err != nil {
		return snapshotGitDirs{}, fmt.Errorf("resolve git dir: %w", err)
	}
	commonDir, err := deps.Git.GitCommonDir(ctx, repoRoot)
	if err != nil {
		return snapshotGitDirs{}, fmt.Errorf("resolve git common dir: %w", err)
	}
	gitDir = strings.TrimSpace(gitDir)
	commonDir = strings.TrimSpace(commonDir)
	if gitDir == "" {
		return snapshotGitDirs{}, fmt.Errorf("git dir is empty in %s", repoRoot)
	}
	if commonDir == "" {
		return snapshotGitDirs{}, fmt.Errorf("git common dir is empty in %s", repoRoot)
	}
	gitPath := gitDir
	if !isAbsPath(gitPath) {
		gitPath = deps.FileSystem.Join(repoRoot, gitDir)
	}
	commonPath := commonDir
	if !isAbsPath(commonPath) {
		commonPath = deps.FileSystem.Join(repoRoot, commonDir)
	}
	info, err := deps.FileSystem.Stat(ctx, gitPath)
	if err != nil {
		return snapshotGitDirs{}, fmt.Errorf("git dir not found in %s: %w", repoRoot, err)
	}
	if info != nil && !info.IsDir() {
		return snapshotGitDirs{}, fmt.Errorf("git dir is not a directory in %s", repoRoot)
	}
	info, err = deps.FileSystem.Stat(ctx, commonPath)
	if err != nil {
		return snapshotGitDirs{}, fmt.Errorf("git common dir not found in %s: %w", repoRoot, err)
	}
	if info != nil && !info.IsDir() {
		return snapshotGitDirs{}, fmt.Errorf("git common dir is not a directory in %s", repoRoot)
	}
	return snapshotGitDirs{
		gitDir:     gitPath,
		commonDir:  commonPath,
		isWorktree: normalizeRepoPath(deps.FileSystem, gitPath) != normalizeRepoPath(deps.FileSystem, commonPath),
	}, nil
}

func cleanupSnapshotWorktrees(ctx context.Context, deps *Dependencies, dstGit string, bc *backupContext) error {
	worktreesDir := deps.FileSystem.Join(dstGit, "worktrees")
	info, err := deps.FileSystem.Stat(ctx, worktreesDir)
	if err != nil {
		if deps.FileSystem.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat worktrees dir: %w", err)
	}
	if info == nil {
		return nil
	}
	if err := deps.FileSystem.RemoveAll(ctx, worktreesDir); err != nil {
		return fmt.Errorf("remove worktrees dir: %w", err)
	}
	bc.vlogf("→ Removed .git/worktrees from snapshot")
	return nil
}

func sanitizeSegment(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b = append(b, r)
		} else {
			b = append(b, '_')
		}
	}
	out := strings.Trim(strings.TrimLeft(string(b), "."), "_- ")
	if out == "" {
		out = "repo"
	}
	return out
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:8]
}

func shortHashN(s string, n int) string {
	if n <= 0 {
		n = 8
	}
	if n > 64 {
		n = 64
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:n]
}

func parseRemote(remote string) (host, owner, repo string) {
	if i := strings.Index(remote, ":"); i != -1 && strings.Contains(remote[:i], "@") && !strings.Contains(remote, "://") {
		left, right := remote[:i], remote[i+1:]
		at := strings.LastIndex(left, "@")
		host = left[at+1:]
		parts := strings.Split(strings.TrimSuffix(right, ".git"), "/")
		if len(parts) >= 2 {
			owner, repo = parts[len(parts)-2], parts[len(parts)-1]
			return host, owner, repo
		}
	}
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		host = u.Host
		p := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		parts := strings.Split(p, "/")
		if len(parts) >= 2 {
			owner, repo = parts[len(parts)-2], parts[len(parts)-1]
			return host, owner, repo
		}
	}
	return "", "", ""
}

func deriveRepoKey(
	ctx context.Context,
	cfg *Config,
	deps *Dependencies,
	repoRoot string,
	bc *backupContext,
) string {
	style := strings.TrimSpace(cfg.RepoKeyStyle)
	if style == "" {
		style = repoKeyStyleAuto
	}

	switch style {
	case repoKeyStyleAuto:
		if key, ok := deriveRepoKeyAuto(ctx, cfg, deps, repoRoot, bc); ok {
			return key
		}
		key := repoKeyNameHash(deps.FileSystem, repoRoot)
		bc.vlogf("→ Repo key (auto: %s): %s", repoKeyStyleNameHash, key)
		return key
	case repoKeyStyleCustom:
		if key, ok := deriveRepoKeyCustom(ctx, deps, repoRoot); ok {
			return key
		}
	case repoKeyStyleRemoteHierarchy:
		if key, ok := deriveRepoKeyRemoteHierarchy(ctx, deps, repoRoot, bc); ok {
			return key
		}
	}

	key := repoKeyNameHash(deps.FileSystem, repoRoot)
	bc.vlogf("→ Repo key (%s): %s", repoKeyStyleNameHash, key)
	return key
}

func deriveRepoKeyAuto(
	ctx context.Context,
	cfg *Config,
	deps *Dependencies,
	repoRoot string,
	bc *backupContext,
) (string, bool) {
	if slug, err := deps.Git.ConfigGet(ctx, repoRoot, "backup.slug"); err == nil && strings.TrimSpace(slug) != "" {
		if key, ok := repoKeyFromSlug(deps.FileSystem, repoRoot, slug); ok {
			bc.vlogf("→ Repo key (auto: slug+repo): %s", key)
			return key, true
		}
	}

	if remote, err := deps.Git.ConfigGet(ctx, repoRoot, "remote.origin.url"); err == nil && remote != "" {
		if key, ok := repoKeyFromRemote(deps.FileSystem, remote, repoRoot, cfg); ok {
			if !cfg.AutoRemoteMerge {
				bc.vlogf("→ Repo key (auto: remote+hash): %s", key)
			} else {
				bc.vlogf("→ Repo key (auto: remote): %s", key)
			}
			return key, true
		}
	}

	return "", false
}

func deriveRepoKeyCustom(ctx context.Context, deps *Dependencies, repoRoot string) (string, bool) {
	slug, err := deps.Git.ConfigGet(ctx, repoRoot, "backup.slug")
	if err != nil || strings.TrimSpace(slug) == "" {
		return "", false
	}
	return repoKeyFromSlug(deps.FileSystem, repoRoot, slug)
}

func deriveRepoKeyRemoteHierarchy(
	ctx context.Context,
	deps *Dependencies,
	repoRoot string,
	bc *backupContext,
) (string, bool) {
	remote, err := deps.Git.ConfigGet(ctx, repoRoot, "remote.origin.url")
	if err != nil || remote == "" {
		return "", false
	}
	h, o, r := parseRemote(remote)
	if h == "" || o == "" || r == "" {
		return "", false
	}
	key := deps.FileSystem.Join(sanitizeSegment(h), sanitizeSegment(o), sanitizeSegment(r))
	bc.vlogf("→ Repo key (remote): %s", key)
	return key, true
}

func repoKeyFromSlug(fs FileSystemPort, repoRoot, slug string) (string, bool) {
	segs := make([]string, 0, 4)
	for _, s := range strings.Split(slug, "/") {
		s = sanitizeSegment(s)
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "", false
	}
	base := sanitizeSegment(fs.Base(repoRoot))
	segs = append(segs, base)
	return fs.Join(segs...), true
}

func repoKeyFromRemote(fs FileSystemPort, remote, repoRoot string, cfg *Config) (string, bool) {
	h, o, r := parseRemote(remote)
	if h == "" || o == "" || r == "" {
		return "", false
	}
	key := fs.Join(sanitizeSegment(h), sanitizeSegment(o), sanitizeSegment(r))
	if !cfg.AutoRemoteMerge {
		key = key + "--" + shortHashN(repoRoot, cfg.RemoteHashLen)
	}
	return key, true
}

func repoKeyNameHash(fs FileSystemPort, repoRoot string) string {
	base := sanitizeSegment(fs.Base(repoRoot))
	return fmt.Sprintf("%s--%s", base, shortHash(repoRoot))
}

func readDevbackIgnore(ctx context.Context, deps *Dependencies, repoRoot string, bc *backupContext) ([]string, error) {
	path := deps.FileSystem.Join(repoRoot, ".devbackignore")
	data, err := deps.FileSystem.ReadFile(ctx, path)
	if err != nil {
		if deps.FileSystem.IsNotExist(err) {
			bc.vlogf("→ No .devbackignore in repo")
			return nil, nil
		}
		return nil, err
	}

	bc.vlogf("→ Found .devbackignore (normalized):")

	excludes := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.ReplaceAll(line, "\r", "")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		excludes = append(excludes, line)
		bc.vlogf("   exclude='%s'", line)
	}
	return excludes, nil
}

func shouldSkip(p string, excludes []string) (bool, string) {
	normalizedPath := strings.ReplaceAll(p, "\\", "/")
	for _, ex := range excludes {
		pattern := strings.ReplaceAll(ex, "\\", "/")
		hasMeta := strings.ContainsAny(pattern, "*?[")
		hasSlash := strings.Contains(pattern, "/")

		if hasMeta {
			if hasSlash {
				if ok, _ := path.Match(pattern, normalizedPath); ok {
					return true, ex
				}
			} else if ok, _ := path.Match(pattern, path.Base(normalizedPath)); ok {
				return true, ex
			}
			continue
		}

		if normalizedPath == pattern || strings.HasPrefix(normalizedPath, pattern+"/") {
			return true, ex
		}
	}
	return false, ""
}

func isPermissionError(fs FileSystemPort, err error) bool {
	if err == nil || fs == nil {
		return false
	}
	return fs.IsPermission(err)
}

func recordCopyError(fs FileSystemPort, path string, err error, result *BackupResult, copyErrors *[]string) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf("%s: %v", path, err)
	*copyErrors = append(*copyErrors, msg)

	if result == nil {
		return
	}
	result.SkippedFiles++
	if isPermissionError(fs, err) {
		result.PermissionErrs = append(result.PermissionErrs, msg)
	} else {
		result.OtherErrors = append(result.OtherErrors, msg)
	}
	result.PartialSuccess = true
}

func copyFile(ctx context.Context, deps *Dependencies, src, dst string, mode int) error {
	if err := deps.FileSystem.Copy(ctx, src, dst); err != nil {
		return err
	}
	if mode != 0 {
		_ = deps.FileSystem.Chmod(ctx, dst, mode&0o777)
	}
	return nil
}

func copyDirRecursive(
	ctx context.Context,
	deps *Dependencies,
	src,
	dst string,
	result *BackupResult,
	bc *backupContext,
) error {
	var copyErrors []string

	walkErr := deps.FileSystem.Walk(ctx, src, func(path string, info FileInfo, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			recordCopyError(deps.FileSystem, path, err, result, &copyErrors)
			return nil
		}

		rel, err := deps.FileSystem.Rel(src, path)
		if err != nil {
			recordCopyError(deps.FileSystem, path, err, result, &copyErrors)
			return nil
		}
		target := deps.FileSystem.Join(dst, rel)

		if info == nil {
			return nil
		}

		copyDirEntry(ctx, deps, path, target, info, result, &copyErrors)
		return nil
	})

	if len(copyErrors) > 0 {
		if bc.verbose {
			bc.warnf("encountered %d issues during copy:", len(copyErrors))
			for _, errMsg := range copyErrors {
				bc.warnf("  %s", errMsg)
			}
		}

		return fmt.Errorf("failed to copy %d item(s)", len(copyErrors))
	}

	return walkErr
}

func copyDirEntry(
	ctx context.Context,
	deps *Dependencies,
	path,
	target string,
	info FileInfo,
	result *BackupResult,
	copyErrors *[]string,
) {
	if info.IsDir() {
		if err := deps.FileSystem.CreateDir(ctx, target, info.Mode()&0o777); err != nil {
			recordCopyError(deps.FileSystem, target, err, result, copyErrors)
		}
		return
	}

	if info.IsSymlink() {
		linkTarget, err := deps.FileSystem.Readlink(ctx, path)
		if err != nil {
			recordCopyError(deps.FileSystem, path, err, result, copyErrors)
			return
		}
		parentDir := deps.FileSystem.Dir(target)
		if err := deps.FileSystem.CreateDir(ctx, parentDir, 0o755); err != nil {
			recordCopyError(deps.FileSystem, parentDir, err, result, copyErrors)
			return
		}
		_ = deps.FileSystem.RemoveAll(ctx, target)
		if err := deps.FileSystem.Symlink(ctx, linkTarget, target); err != nil {
			recordCopyError(deps.FileSystem, target, err, result, copyErrors)
		}
		return
	}

	parentDir := deps.FileSystem.Dir(target)
	if err := deps.FileSystem.CreateDir(ctx, parentDir, 0o755); err != nil {
		recordCopyError(deps.FileSystem, parentDir, err, result, copyErrors)
		return
	}
	if err := copyFile(ctx, deps, path, target, info.Mode()); err != nil {
		recordCopyError(deps.FileSystem, path, err, result, copyErrors)
		return
	}
	if result != nil {
		result.CopiedFiles++
	}
}

func copySelectedFiles(
	ctx context.Context,
	deps *Dependencies,
	paths []string,
	srcRoot,
	dstRoot string,
	result *BackupResult,
	bc *backupContext,
) error {
	if len(paths) == 0 {
		return nil
	}

	workers := runtime.NumCPU() * 2
	jobs := make(chan string, workers*2)
	state := &copySelectedState{
		deps:    deps,
		srcRoot: srcRoot,
		dstRoot: dstRoot,
		result:  result,
		bc:      bc,
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go copySelectedWorker(ctx, jobs, &wg, state)
	}

	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	if len(state.copyErrors) > 0 {
		return fmt.Errorf("failed to copy %d item(s): %w", len(state.copyErrors), state.firstErr)
	}
	return nil
}

type copySelectedState struct {
	deps       *Dependencies
	srcRoot    string
	dstRoot    string
	result     *BackupResult
	bc         *backupContext
	mu         sync.Mutex
	copyErrors []string
	firstErr   error
}

func (s *copySelectedState) recordError(path string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordCopyError(s.deps.FileSystem, path, err, s.result, &s.copyErrors)
	if s.firstErr == nil {
		s.firstErr = err
	}
}

func (s *copySelectedState) recordCopied() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		s.result.CopiedFiles++
	}
}

func copySelectedWorker(
	ctx context.Context,
	jobs <-chan string,
	wg *sync.WaitGroup,
	state *copySelectedState,
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case rel, ok := <-jobs:
			if !ok {
				return
			}
			if err := copySelectedPath(ctx, state, rel); err != nil {
				state.bc.warnf("%v", err)
				state.recordError(rel, err)
			}
		}
	}
}

func copySelectedPath(ctx context.Context, state *copySelectedState, rel string) error {
	src := state.deps.FileSystem.Join(state.srcRoot, rel)
	dst := state.deps.FileSystem.Join(state.dstRoot, rel)

	fi, err := state.deps.FileSystem.Lstat(ctx, src)
	if err != nil {
		return fmt.Errorf("skip '%s': %w", rel, err)
	}
	if fi.IsDir() {
		return createDirForCopy(ctx, state.deps, dst, rel)
	}
	if fi.IsSymlink() {
		return copySelectedSymlink(ctx, state.deps, src, dst, rel)
	}

	if err := createParentDirForCopy(ctx, state.deps, dst, rel); err != nil {
		return err
	}
	if err := copyFile(ctx, state.deps, src, dst, fi.Mode()); err != nil {
		return fmt.Errorf("copy '%s': %w", rel, err)
	}
	state.recordCopied()
	if state.bc.verbose {
		state.bc.vlogf("   COPIED: %s", rel)
	}
	return nil
}

func createDirForCopy(ctx context.Context, deps *Dependencies, dst, rel string) error {
	if err := deps.FileSystem.CreateDir(ctx, dst, 0o755); err != nil {
		return fmt.Errorf("mkdir '%s': %w", rel, err)
	}
	return nil
}

func createParentDirForCopy(ctx context.Context, deps *Dependencies, dst, rel string) error {
	parentRel := deps.FileSystem.Dir(rel)
	if err := deps.FileSystem.CreateDir(ctx, deps.FileSystem.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir '%s': %w", parentRel, err)
	}
	return nil
}

func copySelectedSymlink(ctx context.Context, deps *Dependencies, src, dst, rel string) error {
	if err := createParentDirForCopy(ctx, deps, dst, rel); err != nil {
		return err
	}
	linkTarget, err := deps.FileSystem.Readlink(ctx, src)
	if err != nil {
		return fmt.Errorf("readlink '%s': %w", rel, err)
	}
	_ = deps.FileSystem.RemoveAll(ctx, dst)
	if err := deps.FileSystem.Symlink(ctx, linkTarget, dst); err != nil {
		return fmt.Errorf("symlink '%s': %w", rel, err)
	}
	return nil
}

type snapshot struct {
	DateDir string
	TimeDir string
	Done    string
}

func listSnapshots(ctx context.Context, deps *Dependencies, repoDir string) ([]snapshot, error) {
	var snaps []snapshot

	dateDirs, err := deps.FileSystem.ReadDir(ctx, repoDir)
	if err != nil {
		return nil, err
	}
	for _, d := range dateDirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		if !matchDateDir(name) {
			continue
		}
		dd := deps.FileSystem.Join(repoDir, name)
		timeDirs, err := deps.FileSystem.ReadDir(ctx, dd)
		if err != nil {
			continue
		}
		for _, t := range timeDirs {
			if !t.IsDir() {
				continue
			}
			tname := t.Name()
			if !matchTimeDir(tname) {
				continue
			}
			td := deps.FileSystem.Join(dd, tname)
			donePath := deps.FileSystem.Join(td, ".done")
			if _, err := deps.FileSystem.Stat(ctx, donePath); err == nil {
				snaps = append(snaps, snapshot{DateDir: dd, TimeDir: td, Done: donePath})
			}
		}
	}

	sort.Slice(snaps, func(i, j int) bool { return snaps[i].TimeDir < snaps[j].TimeDir })
	return snaps, nil
}

func matchDateDir(name string) bool {
	if len(name) != 10 {
		return false
	}
	if name[4] != '-' || name[7] != '-' {
		return false
	}
	for _, idx := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if name[idx] < '0' || name[idx] > '9' {
			return false
		}
	}
	return true
}

func matchTimeDir(name string) bool {
	parts := strings.Split(name, "-")
	if len(parts) == 1 {
		if len(name) != 6 {
			return false
		}
		for i := 0; i < 6; i++ {
			if name[i] < '0' || name[i] > '9' {
				return false
			}
		}
		return true
	}
	if len(parts[0]) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if parts[0][i] < '0' || parts[0][i] > '9' {
			return false
		}
	}
	for _, part := range parts[1:] {
		if part == "" {
			return false
		}
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return false
			}
		}
	}
	return true
}

func humanKB(kb int64) string {
	const kbInGB = 1024 * 1024
	if kb >= kbInGB {
		return fmt.Sprintf("%.2f GiB", float64(kb)/float64(kbInGB))
	}
	if kb >= 1024 {
		return fmt.Sprintf("%.2f MiB", float64(kb)/1024.0)
	}
	return fmt.Sprintf("%d KiB", kb)
}

func dirSizeKB(ctx context.Context, deps *Dependencies, root string, bc *backupContext) (int64, error) {
	var total int64
	walkErr := deps.FileSystem.Walk(ctx, root, func(path string, info FileInfo, err error) error {
		if err != nil {
			bc.warnf("walk '%s': %v", path, err)
			return nil
		}
		if info != nil && info.IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return (total + 1023) / 1024, walkErr
}

func removeSnapshot(ctx context.Context, deps *Dependencies, s snapshot, bc *backupContext) {
	bc.logf("[rotate] remove %s", s.TimeDir)
	_ = deps.FileSystem.RemoveAll(ctx, s.TimeDir)
	entries, err := deps.FileSystem.ReadDir(ctx, s.DateDir)
	if err == nil && len(entries) == 0 {
		_ = deps.FileSystem.RemoveAll(ctx, s.DateDir)
	} else if err != nil && !deps.FileSystem.IsNotExist(err) {
		bc.warnf("rotation(date dir): %v", err)
	}
}

func rotateRepo(ctx context.Context, deps *Dependencies, repoDir string, cfg *Config, dryRun bool, bc *backupContext) {
	snaps, err := listSnapshots(ctx, deps, repoDir)
	if err != nil {
		bc.warnf("rotation(list): %v", err)
		return
	}
	alive := make([]bool, len(snaps))
	for i := range alive {
		alive[i] = true
	}

	now := time.Now()
	applyKeepDays(ctx, deps, cfg, dryRun, bc, snaps, alive, now)
	applyKeepCount(ctx, deps, cfg, dryRun, bc, snaps, alive)
	applySizeLimit(ctx, deps, cfg, dryRun, bc, snaps, alive)

	if dryRun {
		bc.logf("ℹ️ Rotation was DRY-RUN (no deletions performed).")
	}
	if bc.verbose {
		logRotationSummary(ctx, deps, repoDir, bc)
	}
}

func applyKeepDays(
	ctx context.Context,
	deps *Dependencies,
	cfg *Config,
	dryRun bool,
	bc *backupContext,
	snaps []snapshot,
	alive []bool,
	now time.Time,
) {
	if cfg.KeepDays <= 0 {
		return
	}
	limit := time.Duration(cfg.KeepDays) * 24 * time.Hour
	for i, s := range snaps {
		if !alive[i] {
			continue
		}
		fi, err := deps.FileSystem.Stat(ctx, s.Done)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > limit {
			bc.logf("[rotate:age] remove %s (older than %dd)", s.TimeDir, cfg.KeepDays)
			if !dryRun {
				removeSnapshot(ctx, deps, s, bc)
			}
			alive[i] = false
		}
	}
}

func applyKeepCount(
	ctx context.Context,
	deps *Dependencies,
	cfg *Config,
	dryRun bool,
	bc *backupContext,
	snaps []snapshot,
	alive []bool,
) {
	if cfg.KeepCount <= 0 {
		return
	}
	live := 0
	for _, ok := range alive {
		if ok {
			live++
		}
	}
	if live <= cfg.KeepCount {
		return
	}
	toRemove := live - cfg.KeepCount
	for i := 0; i < len(snaps) && toRemove > 0; i++ {
		if !alive[i] {
			continue
		}
		bc.logf("[rotate:count] remove %s (exceeds %d)", snaps[i].TimeDir, cfg.KeepCount)
		if !dryRun {
			removeSnapshot(ctx, deps, snaps[i], bc)
		}
		alive[i] = false
		toRemove--
	}
}

func applySizeLimit(
	ctx context.Context,
	deps *Dependencies,
	cfg *Config,
	dryRun bool,
	bc *backupContext,
	snaps []snapshot,
	alive []bool,
) {
	if cfg.MaxTotalGBPerRepo <= 0 || cfg.NoSize {
		return
	}
	kbLimit := int64(cfg.MaxTotalGBPerRepo) * 1024 * 1024
	kbLimit += int64(cfg.SizeMarginMB) * 1024

	sizes := make([]int64, len(snaps))
	var totalKB int64
	for i, s := range snaps {
		if !alive[i] {
			continue
		}
		kb, _ := dirSizeKB(ctx, deps, s.TimeDir, bc)
		sizes[i] = kb
		totalKB += kb
	}
	bc.vlogf("[rotate:size] total=%s limit=%s", humanKB(totalKB), humanKB(kbLimit))

	for i := 0; i < len(snaps) && totalKB > kbLimit; i++ {
		if !alive[i] {
			continue
		}
		bc.logf(
			"[rotate:size] remove %s (total %s > %s)",
			snaps[i].TimeDir,
			humanKB(totalKB),
			humanKB(kbLimit),
		)
		if !dryRun {
			removeSnapshot(ctx, deps, snaps[i], bc)
		}
		totalKB -= sizes[i]
		alive[i] = false
	}
}

func logRotationSummary(ctx context.Context, deps *Dependencies, repoDir string, bc *backupContext) {
	snapsFinal, _ := listSnapshots(ctx, deps, repoDir)
	var totalKB int64
	for _, s := range snapsFinal {
		kb, _ := dirSizeKB(ctx, deps, s.TimeDir, bc)
		totalKB += kb
	}
	bc.logf("[rotate:summary] %d snapshots, total %s", len(snapsFinal), humanKB(totalKB))
}

func buildSnapshotPath(fs FileSystemPort, backupDir, repoKey string, now time.Time) string {
	dateDir := now.Format("2006-01-02")
	timeDir := formatTimeDir(now)
	return fs.Join(backupDir, repoKey, dateDir, timeDir)
}

func formatTimeDir(now time.Time) string {
	return fmt.Sprintf("%s-%09d", now.Format("150405"), now.Nanosecond())
}

func printConfig(cfg *Config, bc *backupContext) {
	bc.vlogf("→ Configuration:")
	bc.vlogf("   Backup directory: %s", cfg.BackupDir)
	bc.vlogf("   Keep count: %d", cfg.KeepCount)
	bc.vlogf("   Keep days: %d", cfg.KeepDays)
	bc.vlogf("   Max total GB per repo: %d", cfg.MaxTotalGBPerRepo)
	bc.vlogf("   Size margin MB: %d", cfg.SizeMarginMB)
	bc.vlogf("   No size check: %t", cfg.NoSize)
	bc.vlogf("   Snapshot time format: HHMMSS-NNNNNNNNN")

	style := strings.TrimSpace(cfg.RepoKeyStyle)
	if style == "" {
		style = repoKeyStyleAuto
	}
	bc.vlogf("   Repo key style: %s", style)

	if style == repoKeyStyleAuto || style == repoKeyStyleRemoteHierarchy {
		bc.vlogf("   Auto remote merge: %t", cfg.AutoRemoteMerge)
		if !cfg.AutoRemoteMerge {
			bc.vlogf("   Remote hash length: %d", cfg.RemoteHashLen)
		}
	}

	bc.vlogf("")
}

func printBackupSummary(result *BackupResult, bc *backupContext) {
	if result.SkippedFiles == 0 && len(result.PermissionErrs) == 0 && len(result.OtherErrors) == 0 {
		return
	}

	bc.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	bc.logf("BACKUP SUMMARY:")

	if result.SkippedFiles > 0 {
		bc.warnf("WARNING: Skipped %d files due to errors", result.SkippedFiles)
	}

	if len(result.PermissionErrs) > 5 {
		bc.warnf("First 5 errors:")
		for i := 0; i < 5 && i < len(result.PermissionErrs); i++ {
			bc.warnf("  - %s", result.PermissionErrs[i])
		}
		bc.warnf("  ... and %d more errors", len(result.PermissionErrs)-5)
	} else if len(result.PermissionErrs) > 0 {
		bc.warnf("Errors:")
		for _, err := range result.PermissionErrs {
			bc.warnf("  - %s", err)
		}
	}

	if len(result.OtherErrors) > 0 {
		bc.warnf("Other errors:")
		for _, err := range result.OtherErrors {
			bc.warnf("  - %s", err)
		}
	}

	if result.PartialSuccess {
		bc.warnf("IMPORTANT: Backup completed with warnings")
		bc.warnf("Some files were not backed up due to errors.")
		bc.warnf("Consider checking permissions or disk space.")
	}

	bc.logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func copyRepoSnapshot(
	ctx context.Context,
	deps *Dependencies,
	repoRoot,
	targetPath string,
	result *BackupResult,
	bc *backupContext,
) error {
	dirs, err := resolveSnapshotGitDirs(ctx, deps, repoRoot)
	if err != nil {
		return err
	}
	srcGit := dirs.commonDir
	dstGit := deps.FileSystem.Join(targetPath, ".git")
	if _, err := deps.FileSystem.Stat(ctx, srcGit); err != nil {
		return fmt.Errorf("git common dir not found: %w", err)
	}
	bc.vlogf("→ Copy .git -> %s", dstGit)
	if err := copyDirRecursive(ctx, deps, srcGit, dstGit, result, bc); err != nil {
		bc.warnf("copy .git encountered issues: %v", err)
		return err
	} else {
		bc.logf("✓ .git copied")
	}
	if err := cleanupSnapshotWorktrees(ctx, deps, dstGit, bc); err != nil {
		return err
	}

	excludes, err := readDevbackIgnore(ctx, deps, repoRoot, bc)
	if err != nil {
		bc.warnf(".devbackignore: %v", err)
	}
	allPaths, err := deps.Git.ListIgnoredUntracked(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("git ls-files: %w", ErrCritical)
	}
	if bc.verbose {
		bc.vlogf("→ Raw ignored/untracked from git: %d", len(allPaths))
		for i, p := range allPaths {
			bc.vlogf("   %4d  %s", i+1, p)
		}
	}
	keep := make([]string, 0, len(allPaths))
	for _, p := range allPaths {
		if skip, ex := shouldSkip(p, excludes); skip {
			bc.vlogf("   SKIP: %s (matched '%s')", p, ex)
			continue
		}
		bc.vlogf("   KEEP: %s", p)
		keep = append(keep, p)
	}

	if err := copySelectedFiles(ctx, deps, keep, repoRoot, targetPath, result, bc); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(keep) > 0 {
		bc.logf("✓ Copied ignored/untracked: %d item(s)", len(keep))
	} else {
		bc.logf("⌘ No ignored/untracked files to copy (after exclusions)")
	}

	return nil
}

func planRepoSnapshot(
	ctx context.Context,
	deps *Dependencies,
	repoRoot,
	targetPath string,
	bc *backupContext,
) (int, error) {
	dirs, err := resolveSnapshotGitDirs(ctx, deps, repoRoot)
	if err != nil {
		return 0, err
	}
	srcGit := dirs.commonDir
	dstGit := deps.FileSystem.Join(targetPath, ".git")
	if _, err := deps.FileSystem.Stat(ctx, srcGit); err != nil {
		return 0, fmt.Errorf("git common dir not found: %w", err)
	}
	bc.logf("Dry run: would copy .git to:%s", dstGit)

	excludes, err := readDevbackIgnore(ctx, deps, repoRoot, bc)
	if err != nil {
		bc.warnf(".devbackignore: %v", err)
	}
	allPaths, err := deps.Git.ListIgnoredUntracked(ctx, repoRoot)
	if err != nil {
		return 0, fmt.Errorf("git ls-files: %w", ErrCritical)
	}
	if bc.verbose {
		bc.vlogf("→ Raw ignored/untracked from git: %d", len(allPaths))
		for i, p := range allPaths {
			bc.vlogf("   %4d  %s", i+1, p)
		}
	}
	keep := make([]string, 0, len(allPaths))
	for _, p := range allPaths {
		if skip, ex := shouldSkip(p, excludes); skip {
			bc.vlogf("   SKIP: %s (matched '%s')", p, ex)
			continue
		}
		bc.vlogf("   KEEP: %s", p)
		keep = append(keep, p)
	}

	if len(keep) > 0 {
		bc.logf("Dry run: would copy ignored/untracked: %d item(s)", len(keep))
	} else {
		bc.logf("Dry run: no ignored/untracked files to copy (after exclusions)")
	}

	return len(keep), nil
}

func handleDryRun(
	ctx context.Context,
	cfg *Config,
	deps *Dependencies,
	repoRoot,
	repoKey string,
	bc *backupContext,
) (*BackupResult, error) {
	if deps.FileSystem == nil || deps.Git == nil {
		return nil, fmt.Errorf("dry run requires filesystem and git adapters: %w", ErrCritical)
	}

	snapshotDir := buildSnapshotPath(deps.FileSystem, cfg.BackupDir, repoKey, time.Now())
	bc.logf("Dry run: backup skipped; would create:%s", snapshotDir)

	if _, err := planRepoSnapshot(ctx, deps, repoRoot, snapshotDir, bc); err != nil {
		return nil, fmt.Errorf("dry run planning failed: %w", ErrCritical)
	}

	repoDir := deps.FileSystem.Join(cfg.BackupDir, repoKey)
	if _, err := deps.FileSystem.Stat(ctx, repoDir); err == nil {
		rotateRepo(ctx, deps, repoDir, cfg, true, bc)
	} else if err != nil && !deps.FileSystem.IsNotExist(err) {
		bc.warnf("dry-run rotation stat '%s': %v", repoDir, err)
	}

	return &BackupResult{}, nil
}

func handleBackupFlow(
	ctx context.Context,
	cfg *Config,
	deps *Dependencies,
	repoRoot,
	repoDir string,
	bc *backupContext,
) (*BackupResult, error) {
	if ctx.Err() != nil {
		return nil, ErrInterrupted
	}
	now := time.Now()
	dateDir := now.Format("2006-01-02")
	targetPath, err := createUniqueSnapshotDir(ctx, deps, repoDir, dateDir, now)
	if err != nil {
		return nil, fmt.Errorf("make target dir: %w", ErrCritical)
	}
	reservePath := deps.FileSystem.Join(targetPath, ".reserve")

	backupSuccessful := false
	defer func() {
		if !backupSuccessful {
			bc.vlogf("cleaning up partial backup: %s", targetPath)
			_ = deps.FileSystem.RemoveAll(ctx, targetPath)
			parentDir := deps.FileSystem.Dir(targetPath)
			entries, err := deps.FileSystem.ReadDir(ctx, parentDir)
			if err == nil && len(entries) == 0 {
				_ = deps.FileSystem.RemoveAll(ctx, parentDir)
			}
		}
	}()

	partial := deps.FileSystem.Join(targetPath, ".partial")
	done := deps.FileSystem.Join(targetPath, ".done")
	if err := deps.FileSystem.WriteFile(ctx, partial, []byte{}, 0o644); err != nil {
		return nil, fmt.Errorf("mark partial: %w", ErrCritical)
	}

	result := &BackupResult{}
	if err := copyRepoSnapshot(ctx, deps, repoRoot, targetPath, result, bc); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrInterrupted
		}
		printBackupSummary(result, bc)
		return nil, fmt.Errorf("backup failed: %w", ErrCritical)
	}

	_ = deps.FileSystem.RemoveAll(ctx, partial)
	if err := deps.FileSystem.WriteFile(ctx, done, []byte{}, 0o644); err != nil {
		return nil, fmt.Errorf("mark done: %w", ErrCritical)
	}
	_ = deps.FileSystem.RemoveAll(ctx, reservePath)
	backupSuccessful = true
	if ctx.Err() != nil {
		return nil, ErrInterrupted
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				bc.warnf("rotation panic: %v", r)
			}
		}()
		rotateRepo(ctx, deps, repoDir, cfg, false, bc)
	}()

	bc.logf("✓ Backup finished → %s", targetPath)

	printBackupSummary(result, bc)
	if result.PartialSuccess {
		return result, ErrCritical
	}
	return result, nil
}

func createUniqueSnapshotDir(
	ctx context.Context,
	deps *Dependencies,
	repoDir,
	dateDir string,
	now time.Time,
) (string, error) {
	baseTimeDir := formatTimeDir(now)
	if err := deps.FileSystem.CreateDir(ctx, deps.FileSystem.Join(repoDir, dateDir), 0o755); err != nil {
		return "", err
	}
	for i := 0; i < 100; i++ {
		timeDir := baseTimeDir
		if i > 0 {
			timeDir = fmt.Sprintf("%s-%02d", baseTimeDir, i)
		}
		targetPath := deps.FileSystem.Join(repoDir, dateDir, timeDir)
		if err := deps.FileSystem.CreateDirExclusive(ctx, targetPath, 0o755); err != nil {
			if deps.FileSystem.IsExist(err) {
				continue
			}
			return "", err
		}
		reservePath := deps.FileSystem.Join(targetPath, ".reserve")
		if err := deps.FileSystem.CreateDirExclusive(ctx, reservePath, 0o700); err != nil {
			_ = deps.FileSystem.RemoveAll(ctx, targetPath)
			if deps.FileSystem.IsExist(err) {
				continue
			}
			return "", err
		}
		return targetPath, nil
	}
	return "", fmt.Errorf("failed to create unique snapshot directory")
}
