// Package events defines the centralized event model and in-process bus.
//
// Every subsystem (collectors, rules, discovery, notifications) emits events
// through the bus. Events are persisted, broadcast to WebSocket clients, and
// feed the rules engine.
package events

import (
	"context"
	"sync"
	"time"
)

// Severity levels.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityError    = "error"
	SeverityCritical = "critical"
)

// Well-known event types.
const (
	TypeSystemBoot         = "system.boot"
	TypeServiceStarted     = "service.started"
	TypeServiceStopped     = "service.stopped"
	TypeContainerRestarted = "container.restarted"
	TypeDeviceJoined       = "device.joined"
	TypeDeviceLeft         = "device.left"
	TypeThresholdExceeded  = "threshold.exceeded"
	TypeThresholdRecovered = "threshold.recovered"
	TypeNotificationSent   = "notification.sent"
	TypeDiscoveryChanged   = "discovery.changed"
	TypeNetworkChanged     = "network.changed"
)

// Event is a single timestamped infrastructure event.
type Event struct {
	ID       string         `json:"id"`
	Time     time.Time      `json:"ts"`
	Type     string         `json:"type"`
	Severity string         `json:"severity"`
	Source   string         `json:"source"`
	Message  string         `json:"message"`
	Data     map[string]any `json:"data,omitempty"`
}

// New creates an event with the current timestamp.
func New(etype, severity, source, message string) *Event {
	return &Event{
		ID:       newID(),
		Time:     time.Now().UTC(),
		Type:     etype,
		Severity: severity,
		Source:   source,
		Message:  message,
		Data:     map[string]any{},
	}
}

// Store persists events. The StateStore implements it.
type Store interface {
	InsertEvent(ctx context.Context, ev *Event) error
}

// Bus fans events out to subscribers (WebSocket hub, rules engine).
type Bus struct {
	mu   sync.RWMutex
	subs map[chan *Event]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: map[chan *Event]struct{}{}}
}

// Subscribe registers a channel that receives every event published after this
// call. The returned unsubscribe func must be called to release the channel.
func (b *Bus) Subscribe() (<-chan *Event, func()) {
	ch := make(chan *Event, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

// Publish delivers an event to all subscribers without blocking.
func (b *Bus) Publish(ev *Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // drop for slow subscribers
		}
	}
}

// Tee is a convenience emitter that persists an event and publishes it.
type Tee struct {
	Store  Store
	Bus    *Bus
	Source string
}

func (t *Tee) Emit(ctx context.Context, ev *Event) {
	if t.Source != "" && ev.Source == "" {
		ev.Source = t.Source
	}
	if t.Store != nil {
		if err := t.Store.InsertEvent(ctx, ev); err != nil {
			// storage failure must not break the live path
		}
	}
	t.Bus.Publish(ev)
}

var idCounter atomicCounter

func newID() string {
	return time.Now().Format("20060102T150405.000000000Z") + "-" + idCounter.next()
}

type atomicCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCounter) next() string {
	c.mu.Lock()
	c.n++
	s := c.n
	c.mu.Unlock()
	return itoa(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [10]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
