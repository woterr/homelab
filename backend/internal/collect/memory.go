package collect

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"homelab/backend/internal/metric"
)

// MemorySample is the parsed snapshot of /proc/meminfo (bytes).
type MemorySample struct {
	Total     int64   `json:"total"`
	Used      int64   `json:"used"`
	Available int64   `json:"available"`
	Cached    int64   `json:"cached"`
	Buffers   int64   `json:"buffers"`
	SwapTotal int64   `json:"swap_total"`
	SwapUsed  int64   `json:"swap_used"`
	UsedPct   float64 `json:"used_pct"`
}

// MemoryCollector reads memory metrics from /proc/meminfo.
type MemoryCollector struct {
	Host Host
	cur  MemorySample
}

func NewMemoryCollector(h Host) *MemoryCollector { return &MemoryCollector{Host: h} }

func (c *MemoryCollector) Name() string { return "memory" }

func (c *MemoryCollector) Read() (MemorySample, error) {
	var m MemorySample
	data, err := os.ReadFile(c.Host.Proc + "/meminfo")
	if err != nil {
		return m, err
	}
	kv := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		kv[strings.TrimSuffix(parts[0], ":")] = v * 1024 // kB -> bytes
	}
	m.Total = kv["MemTotal"]
	m.Available = kv["MemAvailable"]
	m.Cached = kv["Cached"]
	m.Buffers = kv["Buffers"]
	m.SwapTotal = kv["SwapTotal"]
	m.SwapUsed = kv["SwapTotal"] - kv["SwapFree"]
	m.Used = m.Total - m.Available
	if m.Total > 0 {
		m.UsedPct = float64(m.Used) / float64(m.Total) * 100
	}
	c.cur = m
	return m, nil
}

func (c *MemoryCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	m, err := c.Read()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return []metric.Sample{
		metric.New("memory.total_bytes", now, float64(m.Total)),
		metric.New("memory.used_bytes", now, float64(m.Used)),
		metric.New("memory.available_bytes", now, float64(m.Available)),
		metric.New("memory.cached_bytes", now, float64(m.Cached)),
		metric.New("memory.buffers_bytes", now, float64(m.Buffers)),
		metric.New("memory.used_percent", now, m.UsedPct),
		metric.New("memory.swap_total_bytes", now, float64(m.SwapTotal)),
		metric.New("memory.swap_used_bytes", now, float64(m.SwapUsed)),
	}, nil
}

// Snapshot returns the last read memory sample.
func (c *MemoryCollector) Snapshot() MemorySample { return c.cur }

// Memory satisfies MemorySource.
func (c *MemoryCollector) Memory() MemorySample { return c.cur }
