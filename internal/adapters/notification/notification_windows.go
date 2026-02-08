//go:build windows

package notification

import (
	"context"
)

// Send is a stub on Windows (not implemented yet).
func (a *Adapter) Send(ctx context.Context, title, message, sound string) error {
	if ctx.Err() != nil {
		return nil
	}
	if a.logger != nil {
		a.logger.Debug("notifications not implemented on Windows")
	}
	return nil
}
