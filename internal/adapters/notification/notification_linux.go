//go:build linux

package notification

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
)

// Send sends a desktop notification on Linux.
func (a *Adapter) Send(ctx context.Context, title, message, sound string) error {
	if ctx.Err() != nil {
		return nil
	}

	notifyPath, err := exec.LookPath("notify-send")
	if err != nil {
		a.logger.Debug("notification backend not found", slog.Any("err", err))
		return nil
	}

	cmd := exec.CommandContext(ctx, notifyPath, title, message)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if runErr := cmd.Run(); runErr != nil {
		a.logger.Debug("notification failed", slog.Any("err", runErr))
	}
	return nil
}
