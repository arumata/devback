package loghandler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

const (
	colorReset   = "\033[0m"
	colorDim     = "\033[2m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBoldRed = "\033[1;31m"
)

// Options configures the Handler.
type Options struct {
	Level    slog.Level
	UseColor bool
}

// Handler is a compact, optionally colored slog.Handler for CLI output.
type Handler struct {
	w       io.Writer
	opts    Options
	mu      *sync.Mutex
	attrs   []slog.Attr
	groups  []string
	bufPool *sync.Pool
}

// NewHandler creates a new Handler writing to w.
func NewHandler(w io.Writer, opts *Options) *Handler {
	h := &Handler{
		w:  w,
		mu: &sync.Mutex{},
		bufPool: &sync.Pool{
			New: func() any { return new(bytes.Buffer) },
		},
	}
	if opts != nil {
		h.opts = *opts
	}
	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level
}

// Handle formats and writes the log record.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	buf := h.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer h.bufPool.Put(buf)

	h.formatTime(buf, r.Time)
	buf.WriteByte(' ')
	h.formatLevel(buf, r.Level)
	if r.Message != "" {
		buf.WriteByte(' ')
		buf.WriteString(r.Message)
	}

	if len(h.attrs) > 0 || r.NumAttrs() > 0 {
		h.writeAttrs(buf, h.attrs)
		r.Attrs(func(a slog.Attr) bool {
			h.writeAttr(buf, a, h.groups)
			return true
		})
	}

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

// WithAttrs returns a new Handler with the given attributes appended.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()
	for _, a := range attrs {
		h2.attrs = append(h2.attrs, h.resolveAttr(a, h.groups))
	}
	return h2
}

// WithGroup returns a new Handler with the given group name appended.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *Handler) clone() *Handler {
	return &Handler{
		w:       h.w,
		opts:    h.opts,
		mu:      h.mu,
		attrs:   append([]slog.Attr(nil), h.attrs...),
		groups:  append([]string(nil), h.groups...),
		bufPool: h.bufPool,
	}
}

func (h *Handler) formatTime(buf *bytes.Buffer, t time.Time) {
	if h.opts.UseColor {
		buf.WriteString(colorDim)
	}
	hour, min, sec := t.Clock()
	writePad2(buf, hour)
	buf.WriteByte(':')
	writePad2(buf, min)
	buf.WriteByte(':')
	writePad2(buf, sec)
	if h.opts.UseColor {
		buf.WriteString(colorReset)
	}
}

func (h *Handler) formatLevel(buf *bytes.Buffer, level slog.Level) {
	var label string
	var color string
	switch {
	case level >= slog.LevelError:
		label = "ERR"
		color = colorBoldRed
	case level >= slog.LevelWarn:
		label = "WRN"
		color = colorYellow
	case level >= slog.LevelInfo:
		label = "INF"
		color = colorGreen
	default:
		label = "DBG"
		color = colorCyan
	}
	if h.opts.UseColor {
		buf.WriteString(color)
	}
	buf.WriteString(label)
	if h.opts.UseColor {
		buf.WriteString(colorReset)
	}
}

func (h *Handler) writeAttrs(buf *bytes.Buffer, attrs []slog.Attr) {
	for _, a := range attrs {
		h.writeResolvedAttr(buf, a)
	}
}

func (h *Handler) writeAttr(buf *bytes.Buffer, a slog.Attr, groups []string) {
	a = h.resolveAttr(a, groups)
	h.writeResolvedAttr(buf, a)
}

func (h *Handler) resolveAttr(a slog.Attr, groups []string) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return a
	}
	if len(groups) > 0 {
		var b bytes.Buffer
		for _, g := range groups {
			b.WriteString(g)
			b.WriteByte('.')
		}
		b.WriteString(a.Key)
		a.Key = b.String()
	}
	return a
}

func (h *Handler) writeResolvedAttr(buf *bytes.Buffer, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}

	buf.WriteByte(' ')
	if h.opts.UseColor {
		buf.WriteString(colorDim)
	}
	buf.WriteString(a.Key)
	buf.WriteByte('=')
	h.writeValue(buf, a.Value)
	if h.opts.UseColor {
		buf.WriteString(colorReset)
	}
}

func (h *Handler) writeValue(buf *bytes.Buffer, v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if needsQuoting(s) {
			fmt.Fprintf(buf, "%q", s)
		} else {
			buf.WriteString(s)
		}
	case slog.KindGroup:
		attrs := v.Group()
		for i, a := range attrs {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(a.Key)
			buf.WriteByte('=')
			h.writeValue(buf, a.Value)
		}
	default:
		s := fmt.Sprint(v.Any())
		if needsQuoting(s) {
			fmt.Fprintf(buf, "%q", s)
		} else {
			buf.WriteString(s)
		}
	}
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for i := range len(s) {
		c := s[i]
		if c <= ' ' || c == '"' || c == '\\' || c == '=' {
			return true
		}
	}
	return false
}

func writePad2(buf *bytes.Buffer, n int) {
	switch {
	case n < 10:
		buf.WriteByte('0')
		buf.WriteByte(byte('0' + n))
	case n < 100:
		buf.WriteByte(byte('0' + n/10))
		buf.WriteByte(byte('0' + n%10))
	default:
		fmt.Fprintf(buf, "%d", n)
	}
}

// Verify interface compliance at compile time.
var _ slog.Handler = (*Handler)(nil)
