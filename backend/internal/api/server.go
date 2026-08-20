package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"homelab/backend/internal/collect"
	"homelab/backend/internal/config"
	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/notify"
	"homelab/backend/internal/rules"
	"homelab/backend/internal/speedtest"
	"homelab/backend/internal/store"

	"github.com/coder/websocket"
)

// Sources bundles the snapshot providers used by the API. In demo mode these
// point at the DemoHost; in real mode at the live collectors.
type Sources struct {
	System      collect.SystemSource
	CPU         collect.CPUSource
	Memory      collect.MemorySource
	Temperature collect.TemperatureSource
	Network     collect.NetworkSource
	Disk        collect.DiskSource
	Filesystem  collect.FilesystemSource
	Partitions  collect.PartitionSource
	Services    collect.ServiceSource
	Devices     collect.DeviceSource
	Power       collect.PowerSource
}

// Server is the HTTP API for homelab.
type Server struct {
	cfg     *config.Config
	log     *log.Logger
	metrics store.MetricStore
	state   *store.StateStore
	bus     *events.Bus
	tee     *events.Tee
	ruleEng *rules.Engine
	notif   *notify.Manager
	sources Sources
	hub     *Hub

	speedMu     sync.Mutex
	speedResult *speedtest.Result
}

func NewServer(cfg *config.Config, lg *log.Logger, metrics store.MetricStore, state *store.StateStore,
	bus *events.Bus, tee *events.Tee, eng *rules.Engine, nf *notify.Manager, src Sources) *Server {
	return &Server{
		cfg:     cfg,
		log:     lg,
		metrics: metrics,
		state:   state,
		bus:     bus,
		tee:     tee,
		ruleEng: eng,
		notif:   nf,
		sources: src,
		hub:     NewHub(lg),
	}
}

// Hub exposes the WebSocket hub (for the scheduler tick hook).
func (s *Server) Hub() *Hub { return s.hub }

// Handler returns the fully routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc("/api/v1"+pattern, s.withLog(h))
	}

	api("/system", s.handleSystem)
	api("/system/cpu", s.handleCPUSnapshot)
	api("/system/memory", s.handleMemorySnapshot)
	api("/system/temperature", s.handleTemperatureSnapshot)

	api("/network/interfaces", s.handleNetworkInterfaces)
	api("/network/devices", s.handleNetworkDevices)
	api("/network/topology", s.handleNetworkTopology)

	api("/storage", s.handleStorage)
	api("/storage/disks", s.handleStorageDisks)

	api("/services", s.handleServices)
	api("/services/", s.handleServiceDetail)
	api("/power", s.handlePower)

	api("/events", s.handleEvents)

	api("/logs", s.handleLogs)

	api("/metrics/series", s.handleMetricSeries)
	api("/metrics/query", s.handleMetricQuery)
	api("/metrics/latest", s.handleMetricLatest)

	api("/rules", s.handleRules)
	api("/rules/", s.handleRuleDelete)

	api("/settings", s.handleSettings)
	api("/settings/", s.handleSettingsPut)

	api("/notify/providers", s.handleNotifyProviders)
	api("/notify/test", s.handleNotifyTest)
	api("/notify/config", s.handleNotifyConfig)
	api("/notify/config/", s.handleNotifyConfigPut)

	api("/internet", s.handleInternetFuture)
	api("/internet/speed", s.handleInternetSpeed)

	mux.HandleFunc("/api/v1/ws", s.handleWebSocket)
	return mux
}

func (s *Server) withLog(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h(w, r)
		s.log.Debug("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	}
}

// handleLogs returns recent buffered log lines, newest last. Supports ?limit=
// and a minimum ?level= (debug|info|warn|error).
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	buf := s.log.Buffer()
	if buf == nil {
		s.writeJSON(w, http.StatusOK, []log.Entry{})
		return
	}
	s.writeJSON(w, http.StatusOK, buf.Tail(limit, r.URL.Query().Get("level")))
}

// ---- helpers ----

// writeJSON writes a JSON response, logging encode failures so they don't
// silently produce empty 200 responses.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("encode response", "err", err)
	}
}

func (s *Server) writeErr(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// resolveRange parses start/end query params (RFC3339 or unix ms) and falls
// back to a window ending now.
func resolveRange(r *http.Request, fallback time.Duration) (time.Time, time.Time) {
	end := time.Now()
	start := end.Add(-fallback)
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := parseTime(v); err == nil {
			end = t
		}
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := parseTime(v); err == nil {
			start = t
		}
	}
	if start.After(end) {
		start, end = end, start
	}
	return start, end
}

func parseTime(v string) (time.Time, error) {
	if ms, err := parseMS(v); err == nil {
		return time.UnixMilli(ms), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time")
}

func parseMS(v string) (int64, error) {
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return ms, nil
}

// resolutionFor picks a sensible query resolution for a time window.
func resolutionFor(dur time.Duration) time.Duration {
	switch {
	case dur <= 2*time.Hour:
		return 5 * time.Second
	case dur <= 6*time.Hour:
		return 30 * time.Second
	case dur <= 12*time.Hour:
		return time.Minute
	case dur <= 24*time.Hour:
		return 2 * time.Minute
	case dur <= 7*24*time.Hour:
		return 10 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// handleWebSocket upgrades and attaches a client.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c := &wsClient{conn: conn, send: make(chan []byte, 256), subs: map[string]struct{}{}}
	s.hub.add(c)
	s.hub.readLoop(c)
}
