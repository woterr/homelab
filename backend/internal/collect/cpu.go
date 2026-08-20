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
	"homelab/backend/internal/util"
)

// cpuLine holds the breakdown of one cpu row in /proc/stat.
type cpuLine struct {
	user, nice, system, idle, iowait int64
	total                            int64
}

// cpuTicks is a snapshot of /proc/stat cpu counters.
type cpuTicks struct {
	time  time.Time
	lines []cpuLine // index 0 = aggregate cpu, 1..N = per-core
}

// CPUSample carries computed utilization deltas for one collection tick.
type CPUSample struct {
	Total     float64
	PerCore   []float64
	User      float64
	System    float64
	IOWait    float64
	Load1     float64
	Load5     float64
	Load15    float64
	FreqMHz   []float64
	CoreCount int
}

// CPUCollector derives CPU utilization from /proc/stat deltas.
type CPUCollector struct {
	Host Host
	mu   sync.Mutex
	prev *cpuTicks
	cur  *CPUSample
}

func NewCPUCollector(h Host) *CPUCollector { return &CPUCollector{Host: h} }

func (c *CPUCollector) Name() string { return "cpu" }

func (c *CPUCollector) readTicks() (*cpuTicks, error) {
	f, err := os.Open(c.Host.Proc + "/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t := &cpuTicks{time: time.Now()}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			break
		}
		name := fields[0]
		if name == "cpu" || isCpuIndex(name[3:]) {
			vals := toInts(fields[1:])
			if len(vals) < 4 {
				break
			}
			var ln cpuLine
			ln.user = vals[0]
			ln.nice = vals[1]
			ln.system = vals[2]
			ln.idle = vals[3]
			if len(vals) > 4 {
				ln.iowait = vals[4]
			}
			ln.total = sum(vals)
			t.lines = append(t.lines, ln)
		}
	}
	return t, sc.Err()
}

func isCpuIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func toInts(fields []string) []int64 {
	out := make([]int64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return out
		}
		out = append(out, v)
	}
	return out
}

func sum(vals []int64) int64 {
	var s int64
	for _, v := range vals {
		s += v
	}
	return s
}

// delta computes utilization between two ticks.
func delta(prev, cur *cpuTicks) *CPUSample {
	out := &CPUSample{}
	if prev == nil || len(prev.lines) == 0 {
		return out
	}
	count := len(cur.lines)
	if len(prev.lines) < count {
		count = len(prev.lines)
	}
	dt := cur.time.Sub(prev.time).Seconds()
	if dt <= 0 {
		return out
	}
	pct := func(prevL, curL cpuLine, used int64) float64 {
		tt := curL.total - prevL.total
		if tt <= 0 {
			return 0
		}
		return float64(used) / float64(tt) * 100
	}
	busy := func(prevL, curL cpuLine) int64 {
		return (curL.total - curL.idle - curL.iowait) - (prevL.total - prevL.idle - prevL.iowait)
	}
	a0 := cur.lines[0]
	p0 := prev.lines[0]
	out.Total = pct(p0, a0, busy(p0, a0))
	out.User = pct(p0, a0, (a0.user+a0.nice)-(p0.user+p0.nice))
	out.System = pct(p0, a0, a0.system-p0.system)
	out.IOWait = pct(p0, a0, a0.iowait-p0.iowait)
	out.CoreCount = count - 1
	for i := 1; i < count; i++ {
		pc, cc := prev.lines[i], cur.lines[i]
		out.PerCore = append(out.PerCore, pct(pc, cc, busy(pc, cc)))
	}
	return out
}

// loadAndFreq reads load average and per-core current frequency.
func (c *CPUCollector) loadAndFreq() (l1, l5, l15 float64, freq []float64) {
	la := util.ReadFirstLine(c.Host.Proc + "/loadavg")
	parts := strings.Fields(la)
	if len(parts) >= 3 {
		l1 = util.ParseFloatOr(parts[0], 0)
		l5 = util.ParseFloatOr(parts[1], 0)
		l15 = util.ParseFloatOr(parts[2], 0)
	}
	entries, _ := os.ReadDir(c.Host.Sys + "/devices/system/cpu")
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "cpu") || !isCpuIndex(name[3:]) {
			continue
		}
		v := util.ReadFirstLine(c.Host.Sys + "/devices/system/cpu/" + name + "/cpufreq/scaling_cur_freq")
		if v == "" {
			continue
		}
		freq = append(freq, util.ParseFloatOr(v, 0)/1000)
	}
	return
}

func (c *CPUCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	cur, err := c.readTicks()
	if err != nil {
		return nil, err
	}
	d := delta(c.prev, cur)
	l1, l5, l15, freq := c.loadAndFreq()
	d.Load1, d.Load5, d.Load15 = l1, l5, l15
	d.FreqMHz = freq

	c.mu.Lock()
	c.prev = cur
	c.cur = d
	c.mu.Unlock()

	now := cur.time
	out := []metric.Sample{
		metric.New("cpu.utilization", now, d.Total),
		metric.New("cpu.user", now, d.User),
		metric.New("cpu.system", now, d.System),
		metric.New("cpu.iowait", now, d.IOWait),
		metric.New("cpu.load.1", now, l1),
		metric.New("cpu.load.5", now, l5),
		metric.New("cpu.load.15", now, l15),
	}
	for i, v := range d.PerCore {
		out = append(out, metric.NewL("cpu.utilization", map[string]string{"core": strconv.Itoa(i)}, now, v))
	}
	for i, f := range freq {
		out = append(out, metric.NewL("cpu.frequency", map[string]string{"core": strconv.Itoa(i)}, now, f))
	}
	return out, nil
}

// Snapshot returns the latest computed sample.
func (c *CPUCollector) Snapshot() *CPUSample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur == nil {
		return &CPUSample{}
	}
	cp := *c.cur
	cp.PerCore = append([]float64(nil), c.cur.PerCore...)
	cp.FreqMHz = append([]float64(nil), c.cur.FreqMHz...)
	return &cp
}

// CPU satisfies CPUSource.
func (c *CPUCollector) CPU() *CPUSample { return c.Snapshot() }
