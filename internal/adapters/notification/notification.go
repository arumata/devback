package notification

import "log/slog"

// Adapter implements NotificationPort.
type Adapter struct {
	logger *slog.Logger
}

// New creates a new notification adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{logger: logger}
}
