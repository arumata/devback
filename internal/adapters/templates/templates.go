package templates

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/arumata/devback/assets"
	"github.com/arumata/devback/internal/usecase"
)

const (
	templateFilePerm = 0o644
	templateExecPerm = 0o755
)

// Adapter implements TemplatesPort using embedded files.
type Adapter struct {
	logger *slog.Logger
}

// New creates a new templates adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("templates adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// List returns embedded template entries with target permissions.
func (a *Adapter) List(ctx context.Context) ([]usecase.TemplateEntry, error) {
	_ = ctx
	entries, err := fs.ReadDir(assets.GitTemplatesFS, assets.GitTemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	result := make([]usecase.TemplateEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		mode := templateFilePerm
		if isExecutableTemplate(name) {
			mode = templateExecPerm
		}
		result = append(result, usecase.TemplateEntry{Name: name, Mode: mode})
	}

	if len(result) == 0 {
		return nil, errors.New("templates directory is empty")
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// Read returns embedded template content by name.
func (a *Adapter) Read(ctx context.Context, name string) ([]byte, error) {
	return readFromFS(ctx, assets.GitTemplatesFS, assets.GitTemplatesDir, name, "template")
}

// ListRepo returns embedded repo template entries.
func (a *Adapter) ListRepo(ctx context.Context) ([]usecase.TemplateEntry, error) {
	_ = ctx
	entries, err := fs.ReadDir(assets.RepoTemplatesFS, assets.RepoTemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("list repo templates: %w", err)
	}

	result := make([]usecase.TemplateEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		result = append(result, usecase.TemplateEntry{Name: entry.Name(), Mode: templateFilePerm})
	}

	if len(result) == 0 {
		return nil, errors.New("repo templates directory is empty")
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// ReadRepo returns embedded repo template content by name.
func (a *Adapter) ReadRepo(ctx context.Context, name string) ([]byte, error) {
	return readFromFS(ctx, assets.RepoTemplatesFS, assets.RepoTemplatesDir, name, "repo template")
}

func readFromFS(_ context.Context, fsys fs.ReadFileFS, dir, name, label string) ([]byte, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return nil, fmt.Errorf("%s name is empty", label)
	}
	if strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return nil, fmt.Errorf("%s name must not contain path separators", label)
	}

	p := path.Join(dir, clean)
	data, err := fsys.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s %s: %w", label, clean, err)
	}

	return data, nil
}

func isExecutableTemplate(name string) bool {
	switch name {
	case "post-commit", "post-merge", "post-rewrite":
		return true
	default:
		return false
	}
}
