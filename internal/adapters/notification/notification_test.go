package notification

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestAdapterSend_NoError(t *testing.T) {
	t.Setenv("PATH", "")
	adapter := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := adapter.Send(context.Background(), "", "", ""); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAdapterSend_ContextCanceled(t *testing.T) {
	t.Setenv("PATH", "")
	adapter := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapter.Send(ctx, "title", "message", "sound"); err != nil {
		t.Fatalf("expected nil error for canceled context, got %v", err)
	}
}
