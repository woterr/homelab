// Package api implements the REST and WebSocket API for homelab. It is the
// only interface the frontend talks to; it never exposes storage or collector
// internals directly.
package api

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"

	"github.com/coder/websocket"
)

// wsMessage is the envelope for every WebSocket frame.
type wsMessage struct {
	Type  string        `json:"t"`
	TS    int64         `json:"ts,omitempty"`
	Event *events.Event `json:"event,omitempty"`
	Tick  *wsTick       `json:"tick,omitempty"`
}

type wsTick struct {
	TS   int64      `json:"ts"`
	Data []wsSample `json:"data"`
}

type wsSample struct {
	Series string  `json:"s"`
	Value  float64 `json:"v"`
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
	subs map[string]struct{}
}

// Hub fans metric ticks and events out to connected clients. Clients can send
// {"t":"sub","series":[...]} to filter ticks; with no subscriptions they
// receive everything.
type Hub struct {
	log     *log.Logger
	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

func NewHub(lg *log.Logger) *Hub {
	return &Hub{log: lg, clients: map[*wsClient]struct{}{}}
}

// add registers a client and starts its write pump.
func (h *Hub) add(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	go h.writeLoop(c)
}

func (h *Hub) remove(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}

// readLoop consumes client messages (currently used only for subscription
// filtering).
func (h *Hub) readLoop(c *wsClient) {
	defer h.remove(c)
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var msg struct {
			Type   string   `json:"t"`
			Series []string `json:"series"`
		}
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "sub" {
			continue
		}
		c.subs = map[string]struct{}{}
		for _, s := range msg.Series {
			c.subs[s] = struct{}{}
		}
	}
}

func (h *Hub) writeLoop(c *wsClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := c.conn.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// PushEvent broadcasts an event to all clients.
func (h *Hub) PushEvent(ev *events.Event) {
	out, err := json.Marshal(wsMessage{Type: "event", Event: ev, TS: ev.Time.UnixMilli()})
	if err != nil {
		return
	}
	h.broadcast(out, nil)
}

// PushTick broadcasts a compact metric tick, honoring per-client subscriptions.
func (h *Hub) PushTick(ctx context.Context, samples []metric.Sample) {
	if len(samples) == 0 {
		return
	}
	// Build the envelope once; per-client filtering keeps payloads small.
	tick := wsTick{TS: time.Now().UnixMilli(), Data: make([]wsSample, 0, len(samples))}
	for _, s := range samples {
		tick.Data = append(tick.Data, wsSample{Series: metric.Key(s.Series, s.Labels), Value: s.Value})
	}
	base, err := json.Marshal(wsMessage{Type: "tick", TS: tick.TS, Tick: &tick})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if len(c.subs) > 0 {
			// Filter to subscribed series for this client.
			filtered := wsTick{TS: tick.TS}
			for _, s := range tick.Data {
				if _, ok := c.subs[s.Series]; ok {
					filtered.Data = append(filtered.Data, s)
				}
			}
			if len(filtered.Data) == 0 {
				continue
			}
			fm, err := json.Marshal(wsMessage{Type: "tick", TS: tick.TS, Tick: &filtered})
			if err != nil {
				continue
			}
			select {
			case c.send <- fm:
			default: // slow client: drop tick
			}
			continue
		}
		select {
		case c.send <- base:
		default:
		}
	}
}

func (h *Hub) broadcast(msg []byte, _ func(*wsClient) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
		}
	}
}
