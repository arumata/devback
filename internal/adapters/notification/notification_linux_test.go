//go:build linux

package notification

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestAdapterSend_EscapesBodyMarkup pins that the body reaching notify-send is
// escaped while the summary is not. Notification servers advertising
// `body-markup` (KDE Plasma) otherwise drop everything after a raw `<` in a
// repository path, and the notification still looks delivered.
func TestAdapterSend_EscapesBodyMarkup(t *testing.T) {
	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argvPath + "\n"
	// #nosec G306 -- fake backend must be executable; only the owner can access it.
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake notify-send: %v", err)
	}
	t.Setenv("PATH", dir)

	adapter := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := adapter.Send(context.Background(), "DevBack <1>", "~/dev/<repo> & co: Backup failed", ""); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// #nosec G304 -- argvPath is inside the test's private temporary directory.
	raw, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatalf("read captured argv: %v", err)
	}
	want := "DevBack <1>\n~/dev/&lt;repo&gt; &amp; co: Backup failed\n"
	if string(raw) != want {
		t.Fatalf("notify-send argv = %q, want %q", raw, want)
	}
}
