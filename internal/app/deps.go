package app

import (
	"log/slog"

	"github.com/arumata/devback/internal/adapters/config"
	"github.com/arumata/devback/internal/adapters/filesystem"
	"github.com/arumata/devback/internal/adapters/git"
	"github.com/arumata/devback/internal/adapters/lock"
	"github.com/arumata/devback/internal/adapters/notification"
	"github.com/arumata/devback/internal/adapters/process"
	"github.com/arumata/devback/internal/adapters/templates"
	"github.com/arumata/devback/internal/usecase"
)

// NewDefaultDependencies creates dependencies with real adapters where available.
func NewDefaultDependencies(logger *slog.Logger) *usecase.Dependencies {
	if logger == nil {
		panic("default dependencies require logger")
	}
	fsAdapter := filesystem.New(logger)
	configAdapter := config.New(logger)
	gitAdapter := git.New(logger)
	lockAdapter := lock.New(logger)
	notificationAdapter := notification.New(logger)
	processAdapter := process.New(logger)
	templatesAdapter := templates.New(logger)

	return &usecase.Dependencies{
		FileSystem:   fsAdapter,
		Config:       configAdapter,
		Git:          gitAdapter,
		Lock:         lockAdapter,
		Process:      processAdapter,
		Templates:    templatesAdapter,
		Notification: notificationAdapter,
	}
}
