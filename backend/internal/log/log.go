// Package log provides a small leveled logger built on slog, with an optional
// in-memory ring buffer for recent lines (served over the Logs API).
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
	buf *Buffer
}

func New(level slog.Level, out io.Writer) *Logger {
	return NewBuffered(level, out, 0)
}

// NewBuffered builds a logger that also keeps the last cap lines in a ring
// buffer. When cap <= 0 no buffer is retained.
func NewBuffered(level slog.Level, out io.Writer, cap int) *Logger {
	if out == nil {
		out = os.Stderr
	}
	base := slog.NewTextHandler(out, &slog.HandlerOptions{Level: level})
	var h slog.Handler = base
	var buf *Buffer
	if cap > 0 {
		buf = NewBuffer(cap)
		h = &bufferHandler{next: base, buf: buf}
	}
	return &Logger{slog.New(h), buf}
}

func (l *Logger) Buffer() *Buffer { return l.buf }

func (l *Logger) With(attrs ...any) *Logger {
	return &Logger{l.Logger.With(attrs...), l.buf}
}

func (l *Logger) Ctx(ctx context.Context) *Logger {
	if v, ok := ctx.Value(ctxLoggerKey{}).(*Logger); ok {
		return v
	}
	return l
}

type ctxLoggerKey struct{}

func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}
