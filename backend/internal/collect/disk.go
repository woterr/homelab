package collect

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"homelab/backend/internal/metric"
)

// DiskStats carries I/O counters for one block device.
type DiskStats struct {
	Device     string  `json:"device"`
	Model      string  `json:"model,omitempty"`
	ReadBps    float64 `json:"read_bps"`
	WriteBps   float64 `json:"write_bps"`
	ReadIOPS   float64 `json:"read_iops"`
	WriteIOPS  float64 `json:"write_iops"`
	BusyPct    float64 `json:"busy_pct"`
	ReadBytes  uint64  `json:"-"`
	WriteBytes uint64  `json:"-"`
	ReadOps    uint64  `json:"-"`
	WriteOps   uint64  `json:"-"`
	BusyMillis uint64  `json:"-"`
}

type diskCounter struct {
	readSectors, writeSectors uint64
	readOps, writeOps         uint64
	busyMillis                uint64
}

// DiskCollector derives per-device throughput/IOPS/busy from /proc/diskstats.
type DiskCollector struct {
	Host     Host
	mu       sync.Mutex
	prev     map[string]diskCounter
	prevTime time.Time
	cur      []DiskStats
}

func NewDiskCollector(h Host) *DiskCollector { return &DiskCollector{Host: h} }

func (c *DiskCollector) Name() string { return "disk" }

// devices returns the block devices present in /sys/class/block.
func (c *DiskCollector) devices() []string {
	entries, err := os.ReadDir(c.Host.Sys + "/class/block")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		// Skip loop and ram devices and partitions.
		name := e.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "zram") {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (c *DiskCollector) model(dev string) string {
	return readFileLine(c.Host.Sys + "/class/block/" + dev + "/device/model")
}

func (c *DiskCollector) readCounters() (map[string]diskCounter, error) {
	f, err := os.Open(c.Host.Proc + "/diskstats")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cur := map[string]diskCounter{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 14 {
			continue
		}
		dev := fields[2]
		var d diskCounter
		d.readOps = atou(fields[3])
		d.readSectors = atou(fields[5])
		d.writeOps = atou(fields[7])
		d.writeSectors = atou(fields[9])
		d.busyMillis = atou(fields[12])
		cur[dev] = d
	}
	return cur, sc.Err()
}

func atou(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func (c *DiskCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	cur, err := c.readCounters()
	if err != nil {
		return nil, err
	}
	now := time.Now()

	devices := c.devices()
	deltaSec := 5.0
	c.mu.Lock()
	prev := c.prev
	if !c.prevTime.IsZero() {
		if d := now.Sub(c.prevTime).Seconds(); d > 0 {
			deltaSec = d
		}
	}
	c.prev = cur
	c.prevTime = now
	c.mu.Unlock()

	var out []metric.Sample
	stats := []DiskStats{}
	for _, dev := range devices {
		cnt, ok := cur[dev]
		if !ok {
			continue
		}
		var d DiskStats
		d.Device = dev
		d.Model = c.model(dev)
		if p, ok := prev[dev]; ok && deltaSec > 0 {
			sector := uint64(512)
			d.ReadBps = float64(cnt.readSectors-p.readSectors) * float64(sector) / deltaSec
			d.WriteBps = float64(cnt.writeSectors-p.writeSectors) * float64(sector) / deltaSec
			d.ReadIOPS = float64(cnt.readOps-p.readOps) / deltaSec
			d.WriteIOPS = float64(cnt.writeOps-p.writeOps) / deltaSec
			d.BusyPct = float64(cnt.busyMillis-p.busyMillis) / (deltaSec * 1000) * 100
			if d.BusyPct > 100 {
				d.BusyPct = 100
			}
		}
		d.ReadBytes = cnt.readSectors * 512
		d.WriteBytes = cnt.writeSectors * 512
		d.ReadOps = cnt.readOps
		d.WriteOps = cnt.writeOps
		d.BusyMillis = cnt.busyMillis
		stats = append(stats, d)

		labels := map[string]string{"device": dev}
		out = append(out,
			metric.NewL("disk.read_bps", labels, now, d.ReadBps),
			metric.NewL("disk.write_bps", labels, now, d.WriteBps),
			metric.NewL("disk.read_iops", labels, now, d.ReadIOPS),
			metric.NewL("disk.write_iops", labels, now, d.WriteIOPS),
			metric.NewL("disk.busy_percent", labels, now, d.BusyPct),
			metric.NewL("disk.read_total_bytes", labels, now, float64(d.ReadBytes)),
			metric.NewL("disk.write_total_bytes", labels, now, float64(d.WriteBytes)),
		)
	}
	c.cur = stats
	return out, nil
}

// Snapshot returns the latest disk stats.
func (c *DiskCollector) Snapshot() []DiskStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]DiskStats, len(c.cur))
	copy(out, c.cur)
	return out
}

// Disk satisfies DiskSource.
func (c *DiskCollector) Disk() []DiskStats { return c.Snapshot() }
