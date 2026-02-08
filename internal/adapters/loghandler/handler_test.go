package loghandler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2025, 1, 15, 14, 32, 5, 0, time.UTC)
}

func newTestHandler(buf *bytes.Buffer, color bool) *Handler {
	return NewHandler(buf, &Options{
		Level:    slog.LevelDebug,
		UseColor: color,
	})
}

func TestHandle_PlainFormat(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "Starting devback", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "14:32:05 INF Starting devback\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandle_AllLevels(t *testing.T) {
	tests := []struct {
		level slog.Level
		label string
	}{
		{slog.LevelDebug, "DBG"},
		{slog.LevelInfo, "INF"},
		{slog.LevelWarn, "WRN"},
		{slog.LevelError, "ERR"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf, false)
			r := slog.NewRecord(fixedTime(), tt.level, "msg", 0)
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), tt.label) {
				t.Errorf("output %q does not contain level label %q", buf.String(), tt.label)
			}
		})
	}
}

func TestHandle_WithAttributes(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelDebug, "skip hook", 0)
	r.AddAttrs(slog.String("reason", "SKIP_NOT_GIT_REPO"))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "14:32:05 DBG skip hook reason=SKIP_NOT_GIT_REPO\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandle_QuotedStringValue(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelWarn, "failed", 0)
	r.AddAttrs(slog.String("error", "lock is held"))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, `error="lock is held"`) {
		t.Errorf("expected quoted value in %q", got)
	}
}

func TestHandle_EmptyStringQuoted(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "msg", 0)
	r.AddAttrs(slog.String("key", ""))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, `key=""`) {
		t.Errorf("expected empty string to be quoted in %q", got)
	}
}

func TestHandle_ColorFormat(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, true)

	r := slog.NewRecord(fixedTime(), slog.LevelError, "Git adapter not available", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, colorBoldRed) {
		t.Errorf("expected bold red color code in colored output: %q", got)
	}
	if !strings.Contains(got, colorReset) {
		t.Errorf("expected reset code in colored output: %q", got)
	}
	if !strings.Contains(got, colorDim) {
		t.Errorf("expected dim color code for timestamp: %q", got)
	}
	if !strings.Contains(got, "ERR") {
		t.Errorf("expected ERR label: %q", got)
	}
}

func TestHandle_NoColorNoANSI(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if strings.Contains(got, "\033[") {
		t.Errorf("no-color output contains ANSI escape codes: %q", got)
	}
}

func TestWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	logger := slog.New(h).With("component", "git")
	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "running", 0)
	if err := logger.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "component=git") {
		t.Errorf("expected prebound attr in %q", got)
	}
}

func TestWithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	logger := slog.New(h).WithGroup("adapter")
	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "init", 0)
	r.AddAttrs(slog.String("name", "git"))
	if err := logger.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "adapter.name=git") {
		t.Errorf("expected grouped attr in %q", got)
	}
}

func TestWithGroupAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	logger := slog.New(h).WithGroup("request").With("id", "abc")
	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "processed", 0)
	r.AddAttrs(slog.Int("status", 200))
	if err := logger.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "request.id=abc") {
		t.Errorf("expected grouped prebound attr in %q", got)
	}
	if !strings.Contains(got, "request.status=200") {
		t.Errorf("expected grouped record attr in %q", got)
	}
}

func TestEnabled(t *testing.T) {
	h := NewHandler(&bytes.Buffer{}, &Options{Level: slog.LevelWarn})

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DEBUG should not be enabled at WARN level")
	}
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("INFO should not be enabled at WARN level")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("WARN should be enabled at WARN level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("ERROR should be enabled at WARN level")
	}
}

func TestHandle_IntAttr(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "count", 0)
	r.AddAttrs(slog.Int("n", 42))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "14:32:05 INF count n=42\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandle_MultipleAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	r := slog.NewRecord(fixedTime(), slog.LevelInfo, "backup", 0)
	r.AddAttrs(
		slog.String("repo", "myproject"),
		slog.Int("files", 10),
		slog.Bool("dry", true),
	)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "14:32:05 INF backup repo=myproject files=10 dry=true\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandle_TimePadding(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)

	earlyTime := time.Date(2025, 1, 1, 1, 2, 3, 0, time.UTC)
	r := slog.NewRecord(earlyTime, slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "01:02:03") {
		t.Errorf("expected zero-padded time, got %q", got)
	}
}

func TestConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf, false)
	logger := slog.New(h)

	var wg sync.WaitGroup
	const n = 100
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			logger.Info("concurrent", "i", 1)
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
}

func TestWithAttrs_Empty(t *testing.T) {
	h := NewHandler(&bytes.Buffer{}, &Options{Level: slog.LevelDebug})
	h2 := h.WithAttrs(nil)
	if h2 != h {
		t.Error("WithAttrs(nil) should return same handler")
	}
}

func TestWithGroup_Empty(t *testing.T) {
	h := NewHandler(&bytes.Buffer{}, &Options{Level: slog.LevelDebug})
	h2 := h.WithGroup("")
	if h2 != h {
		t.Error("WithGroup empty should return same handler")
	}
}

func TestNewHandler_NilOpts(t *testing.T) {
	h := NewHandler(&bytes.Buffer{}, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.opts.Level != slog.LevelInfo {
		// slog.LevelInfo is the zero value
		if h.opts.Level != 0 {
			t.Errorf("expected zero-value level, got %v", h.opts.Level)
		}
	}
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"simple", false},
		{"has space", true},
		{"has=equals", true},
		{`has"quote`, true},
		{`has\backslash`, true},
		{"tab\there", true},
		{"SKIP_NOT_GIT_REPO", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := needsQuoting(tt.input); got != tt.want {
				t.Errorf("needsQuoting(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
