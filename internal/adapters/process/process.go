package process

import (
	"log/slog"
	"os"
)

// Adapter implements ProcessPort using real process operations
type Adapter struct {
	logger *slog.Logger
}

// New creates a new process adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("process adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// GetPID returns the current process PID
func (a *Adapter) GetPID() int {
	return os.Getpid()
}
