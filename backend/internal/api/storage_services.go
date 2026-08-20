package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/speedtest"
	"homelab/backend/internal/store"
)

// handleStorage returns filesystem capacity, disk I/O and mounted-partition
// snapshots.
func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	fs := []any{}
	if s.sources.Filesystem != nil {
		for _, m := range s.sources.Filesystem.Filesystems() {
			fs = append(fs, m)
		}
	}
	disks := []any{}
	if s.sources.Disk != nil {
		for _, d := range s.sources.Disk.Disk() {
			disks = append(disks, d)
		}
	}
	parts := []any{}
	if s.sources.Partitions != nil {
		for _, p := range s.sources.Partitions.Partitions() {
			parts = append(parts, p)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"filesystems": fs,
		"disks":       disks,
		"partitions":  parts,
	})
}

func (s *Server) handleStorageDisks(w http.ResponseWriter, r *http.Request) {
	if s.sources.Disk == nil {
		s.writeErr(w, http.StatusNotFound, "disk source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.Disk.Disk())
}

// handleServices lists discovered services.
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if s.sources.Services == nil {
		s.writeErr(w, http.StatusNotFound, "service source unavailable")
		return
	}
	svcs, err := s.sources.Services.Services(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, svcs)
}

// handleServiceDetail returns a single service or a structured "logs future
// module" response for the /services/:id/logs variant.
func (s *Server) handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.writeErr(w, http.StatusNotFound, "missing service id")
		return
	}
	id := parts[0]
	if len(parts) > 1 && parts[1] == "logs" {
		// Unified logs are a future module; return a structured empty state.
		s.writeJSON(w, http.StatusOK, map[string]any{
			"service": id,
			"logs":    []any{},
			"module":  "logs",
			"status":  "future",
		})
		return
	}
	if s.sources.Services == nil {
		s.writeErr(w, http.StatusNotFound, "service source unavailable")
		return
	}
	svcs, err := s.sources.Services.Services(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, svc := range svcs {
		if svc.ID == id {
			s.writeJSON(w, http.StatusOK, svc)
			return
		}
	}
	s.writeErr(w, http.StatusNotFound, "service not found")
}

// handleEvents returns persisted events, optionally filtered.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EventFilter{Limit: 200}
	if v := q.Get("limit"); v != "" {
		if n, err := parseMS(v); err == nil && n > 0 {
			f.Limit = int(n)
		}
	}
	if v := q.Get("since"); v != "" {
		if t, err := parseTime(v); err == nil {
			f.Since = t
		}
	}
	if v := q.Get("type"); v != "" {
		f.Types = strings.Split(v, ",")
	}
	if v := q.Get("severity"); v != "" {
		f.Severity = v
	}
	evs, err := s.state.ListEvents(r.Context(), f)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if evs == nil {
		evs = []*events.Event{}
	}
	s.writeJSON(w, http.StatusOK, evs)
}

// handleInternetFuture is a structured "future module" response. It also runs a
// lightweight outbound probe (three well-known endpoints) so the dashboard's
// WAN speedometer has real latency/loss readings rather than static zeros.
func (s *Server) handleInternetFuture(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	targets := []string{
		"https://one.one.one.one/",
		"https://www.gstatic.com/generate_204",
		"https://example.com/",
	}
	client := &http.Client{Timeout: 4 * time.Second}

	type result struct {
		ms float64
		ok bool
	}
	results := make([]result, len(targets))
	var wg sync.WaitGroup
	for i, u := range targets {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			results[i] = result{ms: float64(time.Since(start).Microseconds()) / 1000, ok: resp.StatusCode < 400}
		}(i, u)
	}
	wg.Wait()

	var sum float64
	okCount := 0
	best := 0.0
	for _, res := range results {
		if res.ok {
			sum += res.ms
			okCount++
			if best == 0 || res.ms < best {
				best = res.ms
			}
		}
	}

	latency := 0.0
	loss := 1.0
	status := "down"
	if okCount > 0 {
		latency = best
		loss = float64(len(targets)-okCount) / float64(len(targets))
		status = "ok"
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":          status,
		"module":          "internet",
		"latency_ms":      latency,
		"loss":            loss,
		"probed_at":       time.Now().UTC(),
		"targets":         len(targets),
		"download_mbps":   s.speedValue(func(r *speedtest.Result) float64 { return r.DownloadMbps }),
		"upload_mbps":     s.speedValue(func(r *speedtest.Result) float64 { return r.UploadMbps }),
		"speed_tested_at": s.speedTestedAt(),
	})
}

// handleInternetSpeed triggers a full throughput test and returns the result.
// Only one test runs at a time; concurrent callers wait for the in-flight one.
func (s *Server) handleInternetSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.speedMu.Lock()
	defer s.speedMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Second}
	res := speedtest.Run(ctx, client)
	s.speedResult = &res
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) speedValue(pick func(*speedtest.Result) float64) float64 {
	s.speedMu.Lock()
	defer s.speedMu.Unlock()
	if s.speedResult == nil {
		return 0
	}
	return pick(s.speedResult)
}

func (s *Server) speedTestedAt() *time.Time {
	s.speedMu.Lock()
	defer s.speedMu.Unlock()
	if s.speedResult == nil {
		return nil
	}
	return &s.speedResult.TestedAt
}
