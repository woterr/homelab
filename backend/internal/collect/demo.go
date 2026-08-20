package collect

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"homelab/backend/internal/metric"
	"homelab/backend/internal/store"
)

// DemoHost produces a deterministic, believable simulation of a homelab node.
// It implements the Collector interface plus every Source interface, so the
// API and frontend work identically in --demo mode. Values are smooth
// functions of wall-clock time (seeded per tick), so the dashboard feels live
// without random noise spikes.
type DemoHost struct {
	now func() time.Time
	// fixed simulated identity
	hostname string
	model    string
	cores    int
	memTotal float64
	ipv4     string
	mac      string
	gateway  string
}

func NewDemoHost() *DemoHost {
	return &DemoHost{
		now:      time.Now,
		hostname: "homelab",
		model:    "Simulated Homelab Node",
		cores:    4,
		memTotal: 4096 * 1 << 20, // 4 GiB
		ipv4:     "192.168.1.10",
		mac:      "dc:a6:32:11:22:33",
		gateway:  "192.168.1.1",
	}
}

func (d *DemoHost) Name() string { return "demo" }

// tickSeconds returns fractional seconds since the unix epoch for waveform math.
func (d *DemoHost) t() float64 { return float64(d.now().UnixNano()) / 1e9 }

// wave returns base + amp*sin(2π·t/period + phase), the deterministic driver of
// simulated telemetry.
func wave(t, period, base, amp, phase float64) float64 {
	return base + amp*math.Sin(2*math.Pi*t/period+phase)
}

// noise returns a small deterministic jitter in [-amp, amp].
func (d *DemoHost) noise(amp float64, salt float64) float64 {
	src := rand.New(rand.NewSource(int64(d.t()*10) + int64(salt*1000)))
	return (src.Float64()*2 - 1) * amp
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (d *DemoHost) cpuUtil() float64 {
	return clamp(wave(d.t(), 60, 18, 12, 0)+wave(d.t(), 7, 0, 4, 1.3)+d.noise(2, 1), 2, 92)
}

func (d *DemoHost) memUsed() float64 {
	return clamp(wave(d.t(), 600, 0.42, 0.05, 0.7)+d.noise(0.02, 2), 0.3, 0.75)
}

func (d *DemoHost) rxMbps() float64 {
	return clamp(wave(d.t(), 90, 8, 6, 0.2)+wave(d.t(), 11, 0, 2, 2)+d.noise(0.8, 3), 0.2, 60)
}

func (d *DemoHost) txMbps() float64 {
	return clamp(wave(d.t(), 120, 1.5, 1.2, 1.7)+d.noise(0.3, 4), 0.05, 25)
}

func (d *DemoHost) diskReadMBps() float64 {
	return clamp(wave(d.t(), 150, 0.8, 0.6, 0.9)+d.noise(0.15, 5), 0, 6)
}

func (d *DemoHost) diskWriteMBps() float64 {
	return clamp(wave(d.t(), 180, 0.4, 0.3, 2.1)+d.noise(0.1, 6), 0, 4)
}

func (d *DemoHost) tempC() float64 {
	return clamp(wave(d.t(), 240, 48, 8, 0.5)+d.cpuUtil()*0.18+d.noise(0.5, 7), 36, 78)
}

// Collect returns the full simulated metric batch for one tick.
func (d *DemoHost) Collect(ctx context.Context) ([]metric.Sample, error) {
	now := d.now()
	t := d.t()
	var out []metric.Sample

	// CPU
	total := d.cpuUtil()
	user := total * 0.6
	system := total * 0.3
	iowait := clamp(total*0.1+d.noise(0.5, 8), 0, total*0.4)
	out = append(out,
		metric.New("cpu.utilization", now, total),
		metric.New("cpu.user", now, user),
		metric.New("cpu.system", now, system),
		metric.New("cpu.iowait", now, iowait),
		metric.New("cpu.load.1", now, clamp(total/100*4+d.noise(0.1, 9), 0, 8)),
		metric.New("cpu.load.5", now, clamp(total/100*4, 0, 8)),
		metric.New("cpu.load.15", now, clamp(total/100*4*0.9, 0, 8)),
	)
	for i := 0; i < d.cores; i++ {
		core := clamp(total+d.noise(9, float64(i+10)), 1, 99)
		out = append(out,
			metric.NewL("cpu.utilization", map[string]string{"core": fmt.Sprint(i)}, now, core),
			metric.NewL("cpu.frequency", map[string]string{"core": fmt.Sprint(i)}, now, 1400+200*math.Sin(t/40+float64(i))),
		)
	}
	out = append(out, metric.NewL("temperature", map[string]string{"sensor": "cpu-thermal"}, now, d.tempC()))

	// Memory
	memPct := d.memUsed()
	totalB := d.memTotal
	usedB := totalB * memPct
	cachedB := totalB * 0.3
	availB := totalB - usedB
	out = append(out,
		metric.New("memory.total_bytes", now, totalB),
		metric.New("memory.used_bytes", now, usedB),
		metric.New("memory.available_bytes", now, availB),
		metric.New("memory.cached_bytes", now, cachedB),
		metric.New("memory.buffers_bytes", now, totalB*0.03),
		metric.New("memory.used_percent", now, memPct*100),
		metric.New("memory.swap_total_bytes", now, 2*float64(1<<30)),
		metric.New("memory.swap_used_bytes", now, totalB*0.08),
	)

	// Network
	rxBps := d.rxMbps() * 1e6 / 8
	txBps := d.txMbps() * 1e6 / 8
	out = append(out,
		metric.New("network.link_up", now, 1),
		metric.New("network.link_speed_mbps", now, 1000),
		metric.New("network.primary_rx_bps", now, rxBps),
		metric.New("network.primary_tx_bps", now, txBps),
		metric.NewL("network.rx_bps", map[string]string{"interface": "eth0"}, now, rxBps),
		metric.NewL("network.tx_bps", map[string]string{"interface": "eth0"}, now, txBps),
	)

	// Filesystem
	out = append(out,
		metric.NewL("filesystem.total_bytes", map[string]string{"mount": "/", "device": "/dev/mmcblk0p2", "fstype": "ext4"}, now, 29.7*float64(1<<30)),
		metric.NewL("filesystem.used_bytes", map[string]string{"mount": "/", "device": "/dev/mmcblk0p2", "fstype": "ext4"}, now, 15.4*float64(1<<30)),
		metric.NewL("filesystem.used_percent", map[string]string{"mount": "/", "device": "/dev/mmcblk0p2", "fstype": "ext4"}, now, 52),
		metric.NewL("filesystem.total_bytes", map[string]string{"mount": "/mnt/nas", "device": "/dev/sda1", "fstype": "ext4"}, now, 29.3*float64(1<<30)),
		metric.NewL("filesystem.used_bytes", map[string]string{"mount": "/mnt/nas", "device": "/dev/sda1", "fstype": "ext4"}, now, 0.6*float64(1<<30)),
		metric.NewL("filesystem.used_percent", map[string]string{"mount": "/mnt/nas", "device": "/dev/sda1", "fstype": "ext4"}, now, 2),
	)

	// Disk I/O
	rdBps := d.diskReadMBps() * 1e6
	wrBps := d.diskWriteMBps() * 1e6
	for _, dev := range []string{"mmcblk0", "sda"} {
		out = append(out,
			metric.NewL("disk.read_bps", map[string]string{"device": dev}, now, rdBps),
			metric.NewL("disk.write_bps", map[string]string{"device": dev}, now, wrBps),
			metric.NewL("disk.read_iops", map[string]string{"device": dev}, now, clamp(rdBps/4096, 0, 500)),
			metric.NewL("disk.write_iops", map[string]string{"device": dev}, now, clamp(wrBps/4096, 0, 400)),
			metric.NewL("disk.busy_percent", map[string]string{"device": dev}, now, clamp(wrBps/1e7*100, 0, 85)),
		)
	}

	// Services
	for _, s := range []struct {
		name, kind, status string
		cpu, mem           float64
	}{
		{"filestash", "docker", "running", 3.2, 184 * 1e6},
		{"collabora", "docker", "running", 6.5, 512 * 1e6},
		{"caddy", "systemd", "running", 0.6, 24 * 1e6},
		{"ssh", "systemd", "running", 0.1, 8 * 1e6},
		{"docker", "systemd", "running", 0.8, 60 * 1e6},
		{"smbd", "systemd", "running", 0.4, 18 * 1e6},
	} {
		out = append(out,
			metric.NewL("service.up", map[string]string{"service": s.name, "kind": s.kind}, now, 1),
			metric.NewL("service.cpu_percent", map[string]string{"service": s.name, "kind": s.kind}, now, s.cpu+d.noise(s.cpu*0.3, hashF(s.name))),
			metric.NewL("service.memory_bytes", map[string]string{"service": s.name, "kind": s.kind}, now, s.mem+d.noise(s.mem*0.05, hashF(s.name)+1)),
		)
	}
	out = append(out,
		metric.New("services.running", now, 6),
		metric.New("services.total", now, 7),
	)

	// Power
	out = append(out,
		metric.New("power.available", now, 1),
		metric.New("power.voltage_v", now, 5.1+d.noise(0.02, 12)),
	)

	// Uptime
	out = append(out, metric.New("system.uptime_seconds", now, 3*86400+float64(int64(t)%86400)))
	return out, nil
}

func hashF(s string) float64 {
	var h float64
	for i, r := range s {
		h += float64(r) * float64(i+1)
	}
	return math.Mod(h, 100)
}

// ---- Source implementations ----

func (d *DemoHost) Info() SystemInfo {
	return SystemInfo{
		Hostname:     d.hostname,
		OS:           "Debian GNU/Linux 13 (trixie)",
		Kernel:       "6.12.75+rpt-rpi-v8",
		Architecture: "aarch64",
		Model:        d.model,
		CPUID:        "ARM Cortex-A72 (simulated)",
		Cores:        d.cores,
		Uptime:       3 * 86400,
		BootTime:     time.Now().Add(-3 * 86400).Unix(),
		Server:       "node",
	}
}

func (d *DemoHost) CPU() *CPUSample {
	total := d.cpuUtil()
	cp := &CPUSample{
		Total:     total,
		PerCore:   []float64{},
		User:      total * 0.6,
		System:    total * 0.3,
		IOWait:    total * 0.1,
		Load1:     clamp(total/100*4, 0, 8),
		Load5:     clamp(total/100*4, 0, 8),
		Load15:    clamp(total/100*4*0.9, 0, 8),
		FreqMHz:   []float64{},
		CoreCount: d.cores,
	}
	for i := 0; i < d.cores; i++ {
		cp.PerCore = append(cp.PerCore, clamp(total+d.noise(9, float64(i+10)), 1, 99))
		cp.FreqMHz = append(cp.FreqMHz, 1400)
	}
	return cp
}

func (d *DemoHost) Memory() MemorySample {
	memPct := d.memUsed()
	total := int64(d.memTotal)
	used := int64(float64(total) * memPct)
	return MemorySample{
		Total:     total,
		Used:      used,
		Available: total - used,
		Cached:    int64(float64(total) * 0.3),
		Buffers:   int64(float64(total) * 0.03),
		SwapTotal: 2 * 1 << 30,
		SwapUsed:  int64(float64(total) * 0.08),
		UsedPct:   memPct * 100,
	}
}

func (d *DemoHost) Network() ([]NetInterface, string) {
	ifaces := []NetInterface{
		{
			Name:      "eth0",
			MAC:       d.mac,
			MTU:       1500,
			IPv4:      []string{d.ipv4},
			SpeedMbps: 1000,
			OperState: "up",
			Carrier:   true,
			Up:        true,
			Primary:   true,
			RxBps:     d.rxMbps() * 1e6 / 8,
			TxBps:     d.txMbps() * 1e6 / 8,
		},
		{Name: "wlan0", MAC: "dc:a6:32:44:55:66", OperState: "down", Up: false},
		{Name: "docker0", MAC: "02:42:ac:11:00:01", IPv4: []string{"172.17.0.1"}, OperState: "up", Carrier: true, Up: true},
	}
	return ifaces, d.gateway
}

func (d *DemoHost) Disk() []DiskStats {
	rdBps := d.diskReadMBps() * 1e6
	wrBps := d.diskWriteMBps() * 1e6
	return []DiskStats{
		{Device: "mmcblk0", Model: "SD 32GB", ReadBps: rdBps, WriteBps: wrBps, ReadIOPS: clamp(rdBps/4096, 0, 500), WriteIOPS: clamp(wrBps/4096, 0, 400), BusyPct: clamp(wrBps/1e7*100, 0, 85)},
		{Device: "sda", Model: "USB Flash Drive", ReadBps: rdBps * 0.4, WriteBps: wrBps * 0.5, ReadIOPS: clamp(rdBps/4096, 0, 500) * 0.4, WriteIOPS: clamp(wrBps/4096, 0, 400) * 0.5, BusyPct: clamp(wrBps/2e7*100, 0, 60)},
	}
}

// gib converts a GiB count to bytes at runtime (avoids integer-constant
// conversion errors for fractional values).
func gib(n float64) uint64 { return uint64(n * float64(1<<30)) }

func (d *DemoHost) Filesystems() []FilesystemUsage {
	return []FilesystemUsage{
		{Mountpoint: "/", Device: "/dev/mmcblk0p2", FSType: "ext4", Total: gib(29.7), Used: gib(15.4), Available: gib(14.3), UsedPct: 52},
		{Mountpoint: "/mnt/nas", Device: "/dev/sda1", FSType: "ext4", Total: gib(29.3), Used: gib(0.6), Available: gib(28.7), UsedPct: 2},
	}
}

func (d *DemoHost) Power() PowerSample {
	return PowerSample{Available: true, VoltageV: 5.1, PowerState: "nominal"}
}

func (d *DemoHost) Temperature() []ThermalZone {
	return []ThermalZone{{Name: "cpu-thermal", TempC: d.tempC(), Critical: 85}}
}

// ServiceSource
func (d *DemoHost) Services(ctx context.Context) ([]*store.Service, error) {
	now := time.Now()
	svcs := []*store.Service{
		{ID: "docker.filestash", Name: "filestash", Kind: "docker", Status: "running", Image: "filestash-fts5:arm64", StartedAt: now.Add(-17 * 24 * time.Hour), Restarts: 2, CPU: 3.2, Memory: 184e6},
		{ID: "docker.collabora", Name: "collabora", Kind: "docker", Status: "running", Image: "collabora/code:24.04.10.2.1", StartedAt: now.Add(-17 * 24 * time.Hour), Restarts: 1, CPU: 6.5, Memory: 512e6},
		{ID: "systemd.caddy", Name: "caddy", Kind: "systemd", Status: "running", CPU: 0.6, Memory: 24e6},
		{ID: "systemd.ssh", Name: "ssh", Kind: "systemd", Status: "running", CPU: 0.1, Memory: 8e6},
		{ID: "systemd.docker", Name: "docker", Kind: "systemd", Status: "running", CPU: 0.8, Memory: 60e6},
		{ID: "systemd.smbd", Name: "smbd", Kind: "systemd", Status: "running", CPU: 0.4, Memory: 18e6},
		{ID: "systemd.smartd", Name: "smartd", Kind: "systemd", Status: "inactive"},
	}
	return svcs, nil
}

// DeviceSource
func (d *DemoHost) Devices() []*store.Device {
	now := time.Now()
	base := func(ip, mac, hostname, vendor, dtype string, online bool) *store.Device {
		first := now.Add(-72 * time.Hour)
		last := now
		if !online {
			last = now.Add(-2 * time.Hour)
		}
		return &store.Device{
			MAC:        mac,
			IP:         ip,
			Hostname:   hostname,
			Vendor:     vendor,
			DeviceType: dtype,
			FirstSeen:  first,
			LastSeen:   last,
			Online:     online,
		}
	}
	return []*store.Device{
		base(d.gateway, "a8:da:0c:e7:29:21", "router", "TP-Link", "router", true),
		base("192.168.1.42", "3c:97:0e:11:22:33", "dev-laptop", "Intel", "laptop", true),
		base("192.168.1.77", "94:17:c4:aa:bb:cc", "pixel-7", "Google", "mobile", true),
		base("192.168.1.20", "00:11:32:dd:ee:ff", "nas", "Synology", "nas", true),
		base("192.168.1.10", d.mac, d.hostname, "Raspberry Pi", "server", true),
		base("192.168.1.200", "b8:27:eb:12:34:56", "tv", "Raspberry Pi", "unknown", false),
	}
}
