package store

import (
	"context"
	"testing"
	"time"

	"homelab/backend/internal/log"
	"homelab/backend/internal/metric"
)

func newTestStore(t *testing.T) MetricStore {
	t.Helper()
	ctx := context.Background()
	lg := log.New(LogLevelError, nil)
	s, err := OpenMetricStore(ctx, t.TempDir()+"/test.db", lg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// LogLevelError avoids test noise.
const LogLevelError = 4

func TestAppendAndQuery(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	var samples []metric.Sample
	for i := 0; i < 60; i++ {
		ts := now.Add(-time.Duration(60-i) * time.Second)
		samples = append(samples, metric.New("cpu.utilization", ts, float64(i)))
		samples = append(samples, metric.NewL("temperature", map[string]string{"sensor": "cpu-thermal"}, ts, 40+float64(i)))
	}
	if err := s.Append(ctx, samples); err != nil {
		t.Fatal(err)
	}

	// Labeled series must be distinct from unlabeled.
	pts, err := s.Query(ctx, Query{Series: "cpu.utilization", Start: now.Add(-time.Minute), End: now.Add(time.Second), Resolution: 5 * time.Second})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(pts) == 0 {
		t.Fatal("expected points")
	}

	// Labeled temperature query.
	tpts, err := s.Query(ctx, Query{Series: `temperature{sensor="cpu-thermal"}`, Start: now.Add(-time.Minute), End: now.Add(time.Second), Resolution: 10 * time.Second})
	if err != nil {
		t.Fatalf("temp query: %v", err)
	}
	if len(tpts) == 0 {
		t.Fatal("expected temperature points")
	}

	list, err := s.ListSeries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("expected >=2 series, got %d", len(list))
	}
}

func TestLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 3; i++ {
		_ = s.Append(ctx, []metric.Sample{metric.New("memory.used_percent", now.Add(time.Duration(i)*time.Second), float64(10+i))})
	}
	p, ok, err := s.Latest(ctx, "memory.used_percent")
	if err != nil || !ok {
		t.Fatalf("latest: ok=%v err=%v", ok, err)
	}
	if p.Avg != 12 {
		t.Fatalf("expected 12, got %v", p.Avg)
	}
}

func TestRollupAndTrim(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	var samples []metric.Sample
	for i := 0; i < 3600; i++ {
		ts := now.Add(-time.Duration(3600-i) * time.Second)
		samples = append(samples, metric.New("disk.read_bps", ts, float64(i%100)))
	}
	if err := s.Append(ctx, samples); err != nil {
		t.Fatal(err)
	}

	// Roll everything into coarse buckets.
	if err := s.Rollup(ctx, now.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}

	// Query across the full span at a coarse resolution must still work.
	pts, err := s.Query(ctx, Query{Series: "disk.read_bps", Start: now.Add(-time.Hour), End: now.Add(time.Second), Resolution: 5 * time.Minute})
	if err != nil {
		t.Fatalf("rollup query: %v", err)
	}
	if len(pts) < 10 {
		t.Fatalf("expected >=10 rollup points, got %d", len(pts))
	}

	// Trim older data.
	if err := s.Trim(ctx, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
}
