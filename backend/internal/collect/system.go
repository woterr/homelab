package collect

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"homelab/backend/internal/events"
	"homelab/backend/internal/metric"
	"homelab/backend/internal/util"
)

// SystemInfo describes the host identity.
type SystemInfo struct {
	Hostname     string  `json:"hostname"`
	OS           string  `json:"os"`
	Kernel       string  `json:"kernel"`
	Architecture string  `json:"architecture"`
	Model        string  `json:"model"`
	CPUID        string  `json:"cpu_model"`
	Cores        int     `json:"cores"`
	Uptime       float64 `json:"uptime_seconds"`
	BootTime     int64   `json:"boot_time"`
	Server       string  `json:"server"`
}

// SystemCollector reports identity and uptime as metrics, and detects reboots.
type SystemCollector struct {
	Host   Host
	info   SystemInfo
	uptime float64
	tee    *events.Tee
}

func NewSystemCollector(h Host, tee *events.Tee) *SystemCollector {
	return &SystemCollector{Host: h, tee: tee}
}

func (c *SystemCollector) Name() string { return "system" }

func (c *SystemCollector) Gather() SystemInfo {
	now := time.Now()
	up := util.ParseFloatOr(readFileLine(c.Host.Proc+"/uptime"), 0)
	boot := now.Add(-time.Duration(up * float64(time.Second)))

	info := SystemInfo{
		Hostname:     hostname(),
		OS:           osReleasePretty(),
		Kernel:       kernelVersion(c.Host.Proc),
		Architecture: util.Arch(),
		Model:        deviceTreeModel(c.Host.Proc),
		CPUID:        cpuModel(c.Host.Proc),
		Cores:        runtime.NumCPU(),
		Uptime:       up,
		BootTime:     boot.Unix(),
		Server:       "node",
	}
	return info
}

func (c *SystemCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	info := c.Gather()
	now := time.Now()
	c.info = info

	// Detect reboot: uptime decreased significantly vs previous tick.
	if c.uptime > 0 && info.Uptime < c.uptime-30 && c.tee != nil {
		c.tee.Emit(ctx, events.New(events.TypeSystemBoot, events.SeverityInfo, "system",
			"System boot detected"))
	}
	c.uptime = info.Uptime

	return []metric.Sample{
		metric.New("system.uptime_seconds", now, info.Uptime),
	}, nil
}

// Info returns the last gathered identity snapshot.
func (c *SystemCollector) Info() SystemInfo { return c.info }

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return util.ReadFirstLine("/etc/hostname")
}

func osReleasePretty() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

func kernelVersion(proc string) string {
	return util.ReadFirstLine(proc + "/version")
}

func deviceTreeModel(proc string) string {
	return util.ReadFirstLine(proc + "/device-tree/model")
}

func cpuModel(proc string) string {
	data, err := os.ReadFile(proc + "/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Processor") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func readFileLine(path string) string { return util.ReadFirstLine(path) }
