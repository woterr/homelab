// Package collect defines the modular collector architecture and the V1
// Linux collectors. Each collector implements a small interface and is
// registered with a scheduler; the demo driver produces the same metric
// models for development and demos.
package collect

import (
	"context"
	"sort"
	"sync"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/store"
)

// Host describes paths used to observe the real host. When running inside a
// container these point at bind-mounted /proc, /sys and the Docker socket.
// Root, when set, points at a read-only bind mount of the host filesystem
// root so capacity/usage can be read for host mountpoints that are not present
// in the container's own mount namespace.
type Host struct {
	Proc   string
	Sys    string
	Docker string
	Root   string
}

// Collector produces a batch of normalized samples per tick.
type Collector interface {
	Name() string
	Collect(ctx context.Context) ([]metric.Sample, error)
}

// Registry holds the active collectors in registration order.
type Registry struct {
	items []Collector
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Add(c Collector) *Registry {
	r.items = append(r.items, c)
	return r
}

func (r *Registry) Items() []Collector { return r.items }

// Scheduler ticks all collectors on a fixed interval and appends samples to
// the MetricStore. An optional onTick hook receives the full batch after each
// successful append (used by the WebSocket hub and rules engine).
type Scheduler struct {
	reg      *Registry
	interval time.Duration
	store    store.MetricStore
	tee      *events.Tee
	log      *log.Logger
	onTick   func(ctx context.Context, samples []metric.Sample)
	mu       sync.RWMutex
}

func NewScheduler(reg *Registry, interval time.Duration, s store.MetricStore, tee *events.Tee, lg *log.Logger) *Scheduler {
	return &Scheduler{reg: reg, interval: interval, store: s, tee: tee, log: lg}
}

// SetOnTick registers a callback invoked with each collected batch.
func (s *Scheduler) SetOnTick(fn func(ctx context.Context, samples []metric.Sample)) {
	s.mu.Lock()
	s.onTick = fn
	s.mu.Unlock()
}

// Run blocks until the context is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick runs one collection round: gather samples from every collector in
// parallel (each bounded by the tick interval), then append them in a single
// batched write. Parallelizing avoids one slow collector delaying the others,
// which previously produced "tick overran interval" warnings.
func (s *Scheduler) tick(ctx context.Context) {
	start := time.Now()
	items := s.reg.Items()
	type result struct {
		name    string
		samples []metric.Sample
		err     error
	}
	results := make([]result, len(items))

	var wg sync.WaitGroup
	for i, c := range items {
		wg.Add(1)
		go func(i int, c Collector) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, s.interval)
			defer cancel()
			samples, err := c.Collect(cctx)
			results[i] = result{name: c.Name(), samples: samples, err: err}
		}(i, c)
	}
	wg.Wait()

	var all []metric.Sample
	for _, r := range results {
		if r.err != nil {
			s.log.Warn("collector failed", "collector", r.name, "error", r.err)
			continue
		}
		all = append(all, r.samples...)
	}
	if err := s.store.Append(ctx, all); err != nil {
		s.log.Error("metric append failed", "error", err)
		return
	}
	s.mu.RLock()
	fn := s.onTick
	s.mu.RUnlock()
	if fn != nil {
		fn(ctx, all)
	}
	if dur := time.Since(start); dur > s.interval {
		s.log.Warn("collection tick overran interval", "duration", dur, "collectors", len(items))
	}
}

// sortSamples orders samples by series then time (deterministic tests/rollups).
func sortSamples(samples []metric.Sample) {
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].Series != samples[j].Series {
			return samples[i].Series < samples[j].Series
		}
		return samples[i].Time.Before(samples[j].Time)
	})
}

// scaled returns a float from a "fixed point * scale" convention, else def.
func scaled(n int64, scale float64, def float64) float64 {
	if n <= 0 {
		return def
	}
	return float64(n) / scale
}
