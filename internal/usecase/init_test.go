package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockGitInit struct {
	globalValues map[string]string
	setCalled    bool
	setValue     string
}

func (m *mockGitInit) RepoRoot(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockGitInit) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	return "", nil
}

func (m *mockGitInit) ConfigSet(ctx context.Context, repoPath, key, value string) error {
	return nil
}

func (m *mockGitInit) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	if m.globalValues == nil {
		return "", nil
	}
	return m.globalValues[key], nil
}

func (m *mockGitInit) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	return "", nil
}

func (m *mockGitInit) ConfigSetWorktree(ctx context.Context, repoPath, key, value string) error {
	return nil
}

func (m *mockGitInit) ConfigSetGlobal(ctx context.Context, key, value string) error {
	if m.globalValues == nil {
		m.globalValues = make(map[string]string)
	}
	m.globalValues[key] = value
	m.setCalled = true
	m.setValue = value
	return nil
}

func (m *mockGitInit) GitDir(ctx context.Context, repoPath string) (string, error) {
	return "", nil
}

func (m *mockGitInit) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	return "", nil
}

func (m *mockGitInit) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	return nil, nil
}

func (m *mockGitInit) ListIgnoredUntracked(ctx context.Context, repoPath string) ([]string, error) {
	return nil, nil
}

type fakeConfigPort struct {
	fs        FileSystemPort
	data      map[string]ConfigFile
	saveCalls int
}

func newFakeConfigPort(fs FileSystemPort) *fakeConfigPort {
	return &fakeConfigPort{
		fs:   fs,
		data: make(map[string]ConfigFile),
	}
}

func (f *fakeConfigPort) Load(ctx context.Context, path string) (ConfigFile, error) {
	if cfg, ok := f.data[path]; ok {
		return cfg, nil
	}
	return DefaultConfigFile(), nil
}

func (f *fakeConfigPort) Save(ctx context.Context, path string, cfg ConfigFile) error {
	f.saveCalls++
	if f.data == nil {
		f.data = make(map[string]ConfigFile)
	}
	f.data[path] = cfg
	return f.fs.WriteFile(ctx, path, []byte("config"), 0o644)
}

type fakeTemplatesPort struct {
	entries      []TemplateEntry
	contents     map[string][]byte
	repoEntries  []TemplateEntry
	repoContents map[string][]byte
}

func newFakeTemplatesPort() *fakeTemplatesPort {
	return &fakeTemplatesPort{
		entries: []TemplateEntry{
			{Name: "post-commit", Mode: 0o755},
			{Name: "post-merge", Mode: 0o755},
			{Name: "post-rewrite", Mode: 0o755},
		},
		contents: map[string][]byte{
			"post-commit":  []byte("#!/bin/sh\nDEVBACK=\"__DEVBACK_BIN__\"\nexec \"$DEVBACK\" hook post-commit \"$@\"\n"),
			"post-merge":   []byte("#!/bin/sh\nDEVBACK=\"__DEVBACK_BIN__\"\nexec \"$DEVBACK\" hook post-merge \"$@\"\n"),
			"post-rewrite": []byte("#!/bin/sh\nDEVBACK=\"__DEVBACK_BIN__\"\nexec \"$DEVBACK\" hook post-rewrite \"$@\"\n"),
		},
		repoEntries: []TemplateEntry{
			{Name: "devbackignore", Mode: 0o644},
		},
		repoContents: map[string][]byte{
			"devbackignore": []byte("# test devbackignore\nnode_modules\n"),
		},
	}
}

func (f *fakeTemplatesPort) List(ctx context.Context) ([]TemplateEntry, error) {
	return f.entries, nil
}

func (f *fakeTemplatesPort) Read(ctx context.Context, name string) ([]byte, error) {
	data, ok := f.contents[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *fakeTemplatesPort) ListRepo(ctx context.Context) ([]TemplateEntry, error) {
	return f.repoEntries, nil
}

func (f *fakeTemplatesPort) ReadRepo(ctx context.Context, name string) ([]byte, error) {
	if f.repoContents == nil {
		return nil, os.ErrNotExist
	}
	data, ok := f.repoContents[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

type recordingConfigPort struct {
	loadFunc  func(ctx context.Context, path string) (ConfigFile, error)
	saveFunc  func(ctx context.Context, path string, cfg ConfigFile) error
	saveCalls int
}

func (r *recordingConfigPort) Load(ctx context.Context, path string) (ConfigFile, error) {
	return r.loadFunc(ctx, path)
}

func (r *recordingConfigPort) Save(ctx context.Context, path string, cfg ConfigFile) error {
	r.saveCalls++
	if r.saveFunc != nil {
		return r.saveFunc(ctx, path, cfg)
	}
	return nil
}

func TestInit_Success(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	git := &mockGitInit{}
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	testBinPath := "/usr/local/bin/devback"
	opts := InitOptions{HomeDir: homeDir, BinaryPath: testBinPath, BackupDir: "~/backup"}
	if err := Init(ctx, opts, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config not created: %v", err)
	}

	hooksDir := filepath.Join(homeDir, ".local", "share", "devback", "templates", "hooks")
	hookPath := filepath.Join(hooksDir, "post-commit")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("templates not installed: %v", err)
	}

	hookData, err := fs.ReadFile(ctx, hookPath)
	if err != nil {
		t.Fatalf("read hook file: %v", err)
	}
	if bytes.Contains(hookData, []byte(HookBinaryPlaceholder)) {
		t.Fatal("placeholder was not replaced in installed template")
	}
	if !bytes.Contains(hookData, []byte(testBinPath)) {
		t.Fatalf("binary path not found in installed template, got:\n%s", hookData)
	}

	backupDirExpected := filepath.Join(homeDir, "backup")
	if _, err := os.Stat(backupDirExpected); err != nil {
		t.Fatalf("backup directory not created: %v", err)
	}
	logDirExpected := filepath.Join(homeDir, ".local", "state", "devback", "logs")
	if _, err := os.Stat(logDirExpected); err != nil {
		t.Fatalf("log directory not created: %v", err)
	}

	if !git.setCalled {
		t.Fatal("expected git config to be set")
	}
	expected := filepath.Join(homeDir, ".local", "share", "devback", "templates")
	if git.setValue != expected {
		t.Fatalf("unexpected git templateDir: %s", git.setValue)
	}
}

func TestInit_ExistingConfig_NoForce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        &mockGitInit{},
	}

	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := Init(ctx, InitOptions{HomeDir: homeDir, BinaryPath: "/usr/local/bin/devback"}, deps, logger)
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestInit_Force_BackupConfig(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        &mockGitInit{},
	}

	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("old-config"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fixedTime := time.Date(2026, 1, 22, 10, 11, 12, 0, time.UTC)
	prevNow := initNow
	initNow = func() time.Time { return fixedTime }
	t.Cleanup(func() { initNow = prevNow })

	opts := InitOptions{
		HomeDir:    homeDir,
		Force:      true,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	err := Init(ctx, opts, deps, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	backupPath := configPath + ".bak." + fixedTime.Format(initBackupTimeFormat)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config not recreated: %v", err)
	}

	backupDirExpected := filepath.Join(homeDir, "backup")
	if _, err := os.Stat(backupDirExpected); err != nil {
		t.Fatalf("backup directory not created: %v", err)
	}
	logDirExpected := filepath.Join(homeDir, ".local", "state", "devback", "logs")
	if _, err := os.Stat(logDirExpected); err != nil {
		t.Fatalf("log directory not created: %v", err)
	}
}

func TestInit_DryRun_NoChanges(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	git := &mockGitInit{}
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	opts := InitOptions{
		HomeDir:    homeDir,
		DryRun:     true,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	err := Init(ctx, opts, deps, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	configPath := filepath.Join(homeDir, ".config", "devback", "config.toml")
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no config file, got %v", err)
	}
	hooksDir := filepath.Join(homeDir, ".local", "share", "devback", "templates", "hooks")
	if _, err := os.Stat(filepath.Join(hooksDir, "post-commit")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no templates, got %v", err)
	}
	backupDirExpected := filepath.Join(homeDir, "backup")
	if _, err := os.Stat(backupDirExpected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no backup directory in dry-run, got %v", err)
	}
	logDirExpected := filepath.Join(homeDir, ".local", "state", "devback", "logs")
	if _, err := os.Stat(logDirExpected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no log directory in dry-run, got %v", err)
	}
	if git.setCalled {
		t.Fatal("expected no git config changes")
	}
}

func TestInit_TemplatesOnly_UsesConfigDir(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()

	fs := newTestFileSystem()
	configAdapter := newFakeConfigPort(fs)
	defaultTemplatesDir := filepath.Join(homeDir, ".local", "share", "devback", "templates", "hooks")

	recordingConfig := &recordingConfigPort{
		loadFunc: configAdapter.Load,
	}
	git := &mockGitInit{}
	deps := &Dependencies{
		FileSystem: fs,
		Config:     recordingConfig,
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	opts := InitOptions{HomeDir: homeDir, TemplatesOnly: true, BinaryPath: "/usr/local/bin/devback"}
	err := Init(ctx, opts, deps, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if recordingConfig.saveCalls != 0 {
		t.Fatalf("unexpected config save calls: %d", recordingConfig.saveCalls)
	}
	if git.setCalled {
		t.Fatal("expected no git config changes")
	}

	if _, err := os.Stat(filepath.Join(defaultTemplatesDir, "post-commit")); err != nil {
		t.Fatalf("templates not installed to config dir: %v", err)
	}
}

func TestInit_Force_OverwriteGitTemplateDir(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	git := &mockGitInit{
		globalValues: map[string]string{
			"init.templateDir": "/some/other/path",
		},
	}
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	opts := InitOptions{
		HomeDir:    homeDir,
		Force:      true,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	if err := Init(ctx, opts, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !git.setCalled {
		t.Fatal("expected git config to be set")
	}
	expected := filepath.Join(homeDir, ".local", "share", "devback", "templates")
	if git.setValue != expected {
		t.Fatalf("unexpected git templateDir: got %s, want %s", git.setValue, expected)
	}
}

func TestInit_ConflictGitTemplateDir_NoForce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	git := &mockGitInit{
		globalValues: map[string]string{
			"init.templateDir": "/some/other/path",
		},
	}
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	opts := InitOptions{
		HomeDir:    homeDir,
		Force:      false,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	err := Init(ctx, opts, deps, logger)
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if git.setCalled {
		t.Fatal("expected git config not to be changed")
	}
}

func TestInit_MissingBackupDir(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        &mockGitInit{},
	}

	err := Init(ctx, InitOptions{HomeDir: homeDir, BinaryPath: "/usr/local/bin/devback"}, deps, logger)
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error for missing backup-dir, got %v", err)
	}
}

func TestInit_RepoTemplatesInstalled(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	git := &mockGitInit{}
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        git,
	}

	opts := InitOptions{HomeDir: homeDir, BinaryPath: "/usr/local/bin/devback", BackupDir: "~/backup"}
	if err := Init(ctx, opts, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	repoTemplatesDir := filepath.Join(homeDir, ".local", "share", "devback", "repo-templates")
	devbackignorePath := filepath.Join(repoTemplatesDir, "devbackignore")
	if _, err := os.Stat(devbackignorePath); err != nil {
		t.Fatalf("repo template devbackignore not installed: %v", err)
	}

	// #nosec G304 -- test paths are controlled by the test harness.
	data, err := os.ReadFile(devbackignorePath)
	if err != nil {
		t.Fatalf("read repo template: %v", err)
	}
	if !bytes.Contains(data, []byte("node_modules")) {
		t.Fatal("repo template content unexpected")
	}
}

func TestInit_DryRun_NoRepoTemplates(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	homeDir := t.TempDir()
	fs := newTestFileSystem()
	deps := &Dependencies{
		FileSystem: fs,
		Config:     newFakeConfigPort(fs),
		Templates:  newFakeTemplatesPort(),
		Git:        &mockGitInit{},
	}

	opts := InitOptions{
		HomeDir:    homeDir,
		DryRun:     true,
		BinaryPath: "/usr/local/bin/devback",
		BackupDir:  "~/backup",
	}
	if err := Init(ctx, opts, deps, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	repoTemplatesDir := filepath.Join(homeDir, ".local", "share", "devback", "repo-templates")
	if _, err := os.Stat(filepath.Join(repoTemplatesDir, "devbackignore")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no repo templates in dry-run, got %v", err)
	}
}
