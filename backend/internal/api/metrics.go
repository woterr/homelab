package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"homelab/backend/internal/metric"
	"homelab/backend/internal/store"
)

// handleMetricSeries lists all stored series with metadata and parsed labels.
func (s *Server) handleMetricSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.metrics.ListSeries(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type metaOut struct {
		store.SeriesMeta
		Labels map[string]string `json:"labels"`
	}
	out := make([]metaOut, 0, len(series))
	for _, sm := range series {
		_, labels := metric.ParseKey(sm.Name)
		out = append(out, metaOut{SeriesMeta: sm, Labels: labels})
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleMetricQuery resolves a window of points for one series.
//
//	GET /api/v1/metrics/query?series=cpu.utilization&start=..&end=..&resolution=30s&aggregate=avg
func (s *Server) handleMetricQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	series := q.Get("series")
	if series == "" {
		s.writeErr(w, http.StatusBadRequest, "missing series")
		return
	}
	start, end := resolveRange(r, s.cfg.HistoryWindow)
	if end.Sub(start) < time.Second {
		s.writeErr(w, http.StatusBadRequest, "empty range")
		return
	}
	resolution := resolutionFor(end.Sub(start))
	if v := q.Get("resolution"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			resolution = d
		} else if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			resolution = time.Duration(sec) * time.Second
		}
	}
	agg := store.AggAvg
	if v := q.Get("aggregate"); v != "" {
		switch v {
		case "min":
			agg = store.AggMin
		case "max":
			agg = store.AggMax
		case "sum":
			agg = store.AggSum
		case "count":
			agg = store.AggCount
		}
	}
	points, err := s.metrics.Query(r.Context(), store.Query{
		Series:     series,
		Start:      start,
		End:        end,
		Resolution: resolution,
		Aggregate:  agg,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"series":     series,
		"start":      start,
		"end":        end,
		"resolution": resolution.String(),
		"aggregate":  agg,
		"points":     points,
	})
}

// handleMetricLatest returns the most recent value for one or more series.
// Multiple series can be requested as comma-separated values.
func (s *Server) handleMetricLatest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	raw := q.Get("series")
	if raw == "" {
		s.writeErr(w, http.StatusBadRequest, "missing series")
		return
	}
	out := map[string]any{}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		p, ok, err := s.metrics.Latest(r.Context(), name)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ok {
			out[name] = p
		}
	}
	s.writeJSON(w, http.StatusOK, out)
}
