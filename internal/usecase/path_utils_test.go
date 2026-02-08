package usecase

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	fs := newTestFileSystem()
	if runtime.GOOS == "windows" {
		homeDir := `C:\Users\tester`
		got := normalizePath(fs, ` ~\repo\ `, homeDir)
		want := filepath.Clean(`C:\Users\tester\repo`)
		if got != want {
			t.Fatalf("unexpected normalizePath: got %q, want %q", got, want)
		}
		return
	}

	homeDir := filepath.Clean("/home/tester")
	got := normalizePath(fs, " ~/repo/ ", homeDir)
	want := filepath.Clean("/home/tester/repo")
	if got != want {
		t.Fatalf("unexpected normalizePath: got %q, want %q", got, want)
	}
}

func TestContractHomeDir(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		homeDir string
		sep     byte
		want    string
	}{
		{
			name:    "absolute path under home",
			path:    "/home/tester/projects/repo",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "~/projects/repo",
		},
		{
			name:    "path equal to home",
			path:    "/home/tester",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "~",
		},
		{
			name:    "path not under home",
			path:    "/opt/data/repo",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "/opt/data/repo",
		},
		{
			name:    "path already with tilde",
			path:    "~/projects/repo",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "~/projects/repo",
		},
		{
			name:    "empty path",
			path:    "",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "",
		},
		{
			name:    "empty homeDir",
			path:    "/home/tester/repo",
			homeDir: "",
			sep:     '/',
			want:    "/home/tester/repo",
		},
		{
			name:    "home is prefix but not at boundary",
			path:    "/home/tester2/repo",
			homeDir: "/home/tester",
			sep:     '/',
			want:    "/home/tester2/repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contractHomeDir(tt.path, tt.homeDir, tt.sep)
			if got != tt.want {
				t.Fatalf("contractHomeDir(%q, %q, %q) = %q, want %q", tt.path, tt.homeDir, tt.sep, got, tt.want)
			}
		})
	}
}

func TestNormalizeRepoPath(t *testing.T) {
	fs := newTestFileSystem()
	if runtime.GOOS == "windows" {
		got := normalizeRepoPath(fs, ` C:\repo\sub\ `)
		want := filepath.Clean(`C:\repo\sub`)
		if got != want {
			t.Fatalf("unexpected normalizeRepoPath: got %q, want %q", got, want)
		}
		if normalizeRepoPath(fs, `C:\`) != `C:\` {
			t.Fatalf("expected root to be preserved")
		}
		return
	}

	got := normalizeRepoPath(fs, " /repo//sub/ ")
	want := filepath.Clean("/repo/sub")
	if got != want {
		t.Fatalf("unexpected normalizeRepoPath: got %q, want %q", got, want)
	}
	if normalizeRepoPath(fs, "/") != "/" {
		t.Fatalf("expected root to be preserved")
	}
}
