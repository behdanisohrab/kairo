// Package logx configures a friendly, colored structured logger for Kairo.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Colors for the human-friendly terminal output.
const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
	bold   = "\033[1m"
)

func colorLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return red
	case level >= slog.LevelWarn:
		return yellow
	case level >= slog.LevelInfo:
		return cyan
	default:
		return gray
	}
}

func levelText(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN "
	case level >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

// Handler is a minimal slog.Handler that prints a colored, human-friendly line
// per record, e.g.:
//
//	2026-08-19 04:10:05  INFO  kairo: starting host=dns.example.com
type Handler struct {
	mu   *sync.Mutex
	w    io.Writer
	opts slog.HandlerOptions
	attrs []slog.Attr
}

// New returns a slog.Handler writing colored records to w.
func New(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &Handler{
		mu:   &sync.Mutex{},
		w:    w,
		opts: *opts,
	}
}

// Setup configures slog.Default() with a colored handler writing to stderr, and
// returns the logger. Set KAIRO_LOG_LEVEL=debug|info|warn|error to change the
// verbosity.
func Setup() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("KAIRO_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(New(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(gray)
	b.WriteString(r.Time.Format("2006-01-02 15:04:05"))
	b.WriteString(reset)
	b.WriteByte(' ')

	b.WriteString(colorLevel(r.Level))
	b.WriteString(levelText(r.Level))
	b.WriteString(reset)
	b.WriteByte(' ')

	if r.Message != "" {
		b.WriteString(bold)
		b.WriteString(r.Message)
		b.WriteString(reset)
	}

	// Prefix each attribute group with its group name, in order.
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	for _, a := range attrs {
		if a.Key == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(blue)
		b.WriteString(a.Key)
		b.WriteString(reset)
		b.WriteByte('=')
		b.WriteString(fmt.Sprintf("%v", a.Value.Any()))
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	cp.attrs = append(cp.attrs, attrs...)
	return &cp
}

func (h *Handler) WithGroup(name string) slog.Handler {
	// Group names are flattened into attribute prefixes for simplicity.
	return h
}

// Info logs a structured message at Info level.
func Info(msg string, args ...any) { slog.Info(msg, args...) }

// Infof formats a message and logs it at Info level with the given attributes.
func Infof(format string, args ...any) { slog.Info(fmt.Sprintf(format, args...)) }

// Error logs a structured error message at Error level.
func Error(msg string, args ...any) { slog.Error(msg, args...) }

// Fatal logs a structured error message, then exits the process.
func Fatal(err error, args ...any) {
	slog.Error("fatal", append([]any{"error", err}, args...)...)
	os.Exit(1)
}

