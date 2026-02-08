//nolint:gci,gofumpt
package app

import (
	"log/slog"
	"testing"

	"github.com/arumata/devback/internal/adapters/config"
	"github.com/arumata/devback/internal/adapters/filesystem"
	"github.com/arumata/devback/internal/adapters/git"
	"github.com/arumata/devback/internal/adapters/lock"
	"github.com/arumata/devback/internal/adapters/process"
	"github.com/arumata/devback/internal/adapters/templates"
)

func TestNewDefaultDependencies(t *testing.T) {
	deps := NewDefaultDependencies(slog.Default())

	if deps == nil {
		t.Fatal("Expected Dependencies to be created, got nil")
	}

	if deps.FileSystem == nil {
		t.Error("Expected FileSystem adapter to be set")
	}

	if deps.Config == nil {
		t.Error("Expected Config adapter to be set")
	}

	if deps.Git == nil {
		t.Error("Expected Git adapter to be set")
	}

	if deps.Lock == nil {
		t.Error("Expected Lock adapter to be set")
	}

	if deps.Process == nil {
		t.Error("Expected Process adapter to be set")
	}

	if deps.Templates == nil {
		t.Error("Expected Templates adapter to be set")
	}

	// Verify actual adapter types.
	if _, ok := deps.FileSystem.(*filesystem.Adapter); !ok {
		t.Error("Expected FileSystem to be filesystem.Adapter")
	}

	if _, ok := deps.Config.(*config.Adapter); !ok {
		t.Error("Expected Config to be config.Adapter")
	}

	if _, ok := deps.Git.(*git.Adapter); !ok {
		t.Error("Expected Git to be git.Adapter")
	}

	if _, ok := deps.Lock.(*lock.Adapter); !ok {
		t.Error("Expected Lock to be lock.Adapter")
	}

	if _, ok := deps.Process.(*process.Adapter); !ok {
		t.Error("Expected Process to be process.Adapter")
	}

	if _, ok := deps.Templates.(*templates.Adapter); !ok {
		t.Error("Expected Templates to be templates.Adapter")
	}
}

func BenchmarkNewDefaultDependencies(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deps := NewDefaultDependencies(slog.Default())
		if deps == nil {
			b.Fatal("Expected Dependencies to be created, got nil")
		}
	}
}
