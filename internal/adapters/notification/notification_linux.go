//go:build linux

package notification

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strings"
)

// bodyMarkupEscaper escapes the notification body for notification servers that
// advertise the freedesktop `body-markup` capability (KDE Plasma among them).
// There the body is parsed as markup, so a raw `<` in a repository path makes
// the server read the rest as a broken tag and drop everything after it, with no
// error reported anywhere: the notification still looks delivered. Our body is
// data, never intentional markup. The summary is plain text and stays as is,
// where entities would be displayed literally.
//
//nolint:gochecknoglobals // Immutable replacer is safe to share across concurrent notifications.
var bodyMarkupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

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

	cmd := exec.CommandContext(ctx, notifyPath, title, bodyMarkupEscaper.Replace(message))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if runErr := cmd.Run(); runErr != nil {
		a.logger.Debug("notification failed", slog.Any("err", runErr))
	}
	return nil
}
