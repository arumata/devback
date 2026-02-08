//go:build darwin

package notification

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
)

// Send sends a desktop notification on macOS.
func (a *Adapter) Send(ctx context.Context, title, message, sound string) error {
	if ctx.Err() != nil {
		return nil
	}

	notifierPath, err := exec.LookPath("terminal-notifier")
	if err == nil {
		args := []string{"-title", title, "-message", message}
		if sound != "" {
			args = append(args, "-sound", sound)
		}
		cmd := exec.CommandContext(ctx, notifierPath, args...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if runErr := cmd.Run(); runErr != nil {
			a.logger.Debug("notification failed", slog.Any("err", runErr))
		}
		return nil
	}

	if ctx.Err() != nil {
		return nil
	}

	script := buildAppleScriptNotification(title, message)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if runErr := cmd.Run(); runErr != nil {
		a.logger.Debug("notification failed", slog.Any("err", runErr))
	}
	return nil
}

func buildAppleScriptNotification(title, message string) string {
	escapedTitle := escapeAppleScriptString(title)
	escapedMessage := escapeAppleScriptString(message)
	return fmt.Sprintf("display notification \"%s\" with title \"%s\"", escapedMessage, escapedTitle)
}

func escapeAppleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", " ")
	escaped = strings.ReplaceAll(escaped, "\r", " ")
	escaped = strings.ReplaceAll(escaped, "\t", " ")
	return escaped
}
