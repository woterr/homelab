package log

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Entry is one buffered log line, ready for JSON serialization over the API.
type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// Buffer is a fixed-capacity ring buffer of recent log entries. It is safe for
// concurrent use: the slog handler pushes from collector goroutines while API
// handlers read concurrently.
type Buffer struct {
	mu     sync.Mutex
	cap    int
	items  []Entry
	next   int
	filled bool
}

func NewBuffer(capacity int) *Buffer {
	return &Buffer{cap: capacity, items: make([]Entry, capacity)}
}

func (b *Buffer) Push(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[b.next] = e
	b.next = (b.next + 1) % b.cap
	if b.next == 0 {
		b.filled = true
	}
}

var levelRank = map[string]int{
	"DEBUG": 0,
	"INFO":  1,
	"WARN":  2,
	"ERROR": 3,
}

// Tail returns the most recent entries, oldest first, newest last. When a
// minimum level is given (e.g. "warn"), lower-severity lines are dropped.
func (b *Buffer) Tail(limit int, level string) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.next == 0 && !b.filled {
		return nil
	}
	count := b.next
	start := 0
	if b.filled {
		count = b.cap
		start = b.next
	}
	minRank, ok := levelRank[level]
	keep := func(e Entry) bool {
		return !ok || levelRank[e.Level] >= minRank
	}
	out := make([]Entry, 0, min(count, limit))
	for i := 0; i < count; i++ {
		e := b.items[(start+i)%b.cap]
		if !keep(e) {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// bufferHandler tees slog records into the ring buffer before delegating to the
// underlying handler (stderr text output).
type bufferHandler struct {
	next slog.Handler
	buf  *Buffer
}

func (h *bufferHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *bufferHandler) Handle(ctx context.Context, r slog.Record) error {
	e := Entry{
		Time:    r.Time.Format(time.RFC3339),
		Level:   r.Level.String(),
		Message: r.Message,
	}
	if r.NumAttrs() > 0 {
		e.Attrs = make(map[string]any, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			e.Attrs[a.Key] = a.Value.Any()
			return true
		})
	}
	h.buf.Push(e)
	return h.next.Handle(ctx, r)
}

func (h *bufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &bufferHandler{next: h.next.WithAttrs(attrs), buf: h.buf}
}

func (h *bufferHandler) WithGroup(name string) slog.Handler {
	return &bufferHandler{next: h.next.WithGroup(name), buf: h.buf}
}
