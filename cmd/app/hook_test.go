package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/arumata/devback/internal/adapters/noop"
	"github.com/arumata/devback/internal/usecase"
)

type mockGitPort struct {
	noop.Adapter
	worktreeConfig map[string]string
	localConfig    map[string]string
	globalConfig   map[string]string
	repoRoot       string
	gitDir         string
	commonDir      string
	repoRootErr    error
	gitDirErr      error
	commonDirErr   error
}

func (m *mockGitPort) ConfigGetWorktree(ctx context.Context, repoPath, key string) (string, error) {
	return lookupConfig(m.worktreeConfig, key)
}

func (m *mockGitPort) ConfigGet(ctx context.Context, repoPath, key string) (string, error) {
	return lookupConfig(m.localConfig, key)
}

func (m *mockGitPort) ConfigGetGlobal(ctx context.Context, key string) (string, error) {
	return lookupConfig(m.globalConfig, key)
}

func (m *mockGitPort) RepoRoot(ctx context.Context) (string, error) {
	if m.repoRootErr != nil {
		return "", m.repoRootErr
	}
	return m.repoRoot, nil
}

func (m *mockGitPort) GitDir(ctx context.Context, repoPath string) (string, error) {
	if m.gitDirErr != nil {
		return "", m.gitDirErr
	}
	return m.gitDir, nil
}

func (m *mockGitPort) GitCommonDir(ctx context.Context, repoPath string) (string, error) {
	if m.commonDirErr != nil {
		return "", m.commonDirErr
	}
	return m.commonDir, nil
}

type mockFileSystem struct {
	noop.Adapter
	files map[string]mockFileInfo
	reads map[string][]byte
}

type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() int          { return 0 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) IsSymlink() bool    { return false }
func (m mockFileInfo) IsRegular() bool    { return !m.isDir }
func (m mockFileInfo) Sys() interface{}   { return nil }

func (m *mockFileSystem) Join(elements ...string) string {
	return filepath.Join(elements...)
}

func (m *mockFileSystem) Stat(ctx context.Context, path string) (usecase.FileInfo, error) {
	if info, ok := m.files[path]; ok {
		return info, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystem) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if data, ok := m.reads[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func lookupConfig(values map[string]string, key string) (string, error) {
	if values == nil {
		return "", errors.New("not found")
	}
	value, ok := values[key]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func TestParseHookBool(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"YES", true},
		{"on", true},
		{"false", false},
		{"", false},
		{"0", false},
	}

	for _, tc := range cases {
		if got := parseHookBool(tc.value); got != tc.want {
			t.Fatalf("parseHookBool(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestReadRepoConfig_Precedence(t *testing.T) {
	ctx := context.Background()

	git := &mockGitPort{
		worktreeConfig: map[string]string{"backup.enabled": " true "},
		localConfig:    map[string]string{"backup.enabled": "false"},
		globalConfig:   map[string]string{"backup.enabled": "false"},
	}
	if got := readRepoConfig(ctx, git, "."); got != "true" {
		t.Fatalf("expected worktree config, got %q", got)
	}

	git.worktreeConfig = map[string]string{"backup.enabled": "   "}
	if got := readRepoConfig(ctx, git, "."); got != "false" {
		t.Fatalf("expected local config, got %q", got)
	}

	git.localConfig = map[string]string{}
	git.globalConfig = map[string]string{"backup.enabled": "on"}
	if got := readRepoConfig(ctx, git, "."); got != "on" {
		t.Fatalf("expected global config, got %q", got)
	}
}

func TestReadBackupEnabled(t *testing.T) {
	ctx := context.Background()
	git := &mockGitPort{
		globalConfig: map[string]string{"backup.enabled": "true"},
	}
	if !readBackupEnabled(ctx, git, ".") {
		t.Fatal("expected backup to be enabled")
	}

	git = &mockGitPort{
		localConfig:  map[string]string{"backup.enabled": "false"},
		globalConfig: map[string]string{"backup.enabled": "true"},
	}
	if readBackupEnabled(ctx, git, ".") {
		t.Fatal("expected local config to disable backup")
	}
}

func TestNormalizeGitDir(t *testing.T) {
	abs := filepath.Join(string(os.PathSeparator), "repo", ".git")
	if got := normalizeGitDir("/repo", abs); got != abs {
		t.Fatalf("expected absolute path to stay same, got %q", got)
	}

	rel := filepath.Join(".git", "..", ".git")
	if got := normalizeGitDir("/repo", rel); got != filepath.Clean(filepath.Join("/repo", rel)) {
		t.Fatalf("unexpected normalized path: %q", got)
	}
}

func TestIsRebaseInProgress(t *testing.T) {
	ctx := context.Background()
	gitDir := filepath.Join("repo", ".git")

	cases := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"no rebase", nil, false},
		{"rebase-merge", []string{filepath.Join(gitDir, "rebase-merge")}, true},
		{"rebase-apply", []string{filepath.Join(gitDir, "rebase-apply")}, true},
		{"rebase-head", []string{filepath.Join(gitDir, "REBASE_HEAD")}, true},
	}

	for _, tc := range cases {
		files := make(map[string]mockFileInfo)
		for _, path := range tc.paths {
			files[path] = mockFileInfo{name: filepath.Base(path)}
		}
		fs := &mockFileSystem{files: files}
		got, err := isRebaseInProgress(ctx, fs, gitDir)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsRebaseInProgress_Errors(t *testing.T) {
	ctx := context.Background()
	if _, err := isRebaseInProgress(ctx, nil, ".git"); err == nil {
		t.Fatal("expected error for nil filesystem")
	}
	fs := &mockFileSystem{}
	if _, err := isRebaseInProgress(ctx, fs, " "); err == nil {
		t.Fatal("expected error for empty git dir")
	}
}

func TestIsDebounceActive(t *testing.T) {
	now := time.Date(2026, 2, 6, 12, 0, 0, 0, time.UTC)
	stampPath := filepath.Join("repo", ".git", stampFileName)

	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"missing", nil, false},
		{"invalid", []byte("oops"), false},
		{"old", []byte("0"), false},
		{"recent", []byte(strconv.FormatInt(now.Add(-30*time.Second).Unix(), 10)), true},
	}

	for _, tc := range cases {
		fs := &mockFileSystem{reads: map[string][]byte{}}
		if tc.data != nil {
			fs.reads[stampPath] = tc.data
		}
		got, err := isDebounceActive(context.Background(), fs, stampPath, now)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
