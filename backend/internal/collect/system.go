package collect

import (
	"context"
	"os"
	"runtime"
	"strconv"
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
	SoC          string  `json:"soc,omitempty"`
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
		SoC:          socName(c.Host.Proc),
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

// socName extracts the Broadcom SoC identifier (e.g. BCM2711) from the device
// tree "compatible" property, which the firmware sets to brcm,bcm2711. Unlike
// /proc/cpuinfo's misleading "Hardware: BCM2835" line this is per-board accurate.
func socName(proc string) string {
	data, err := os.ReadFile(proc + "/device-tree/compatible")
	if err != nil {
		return ""
	}
	for _, tok := range strings.Split(string(data), "\x00") {
		tok = strings.TrimSpace(tok)
		if i := strings.Index(tok, "bcm"); i >= 0 && i+3 < len(tok) {
			if _, err := strconv.Atoi(tok[i+3:]); err == nil {
				return strings.ToUpper(tok[i:])
			}
		}
	}
	return ""
}

func cpuModel(proc string) string {
	data, err := os.ReadFile(proc + "/cpuinfo")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Processor") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	// aarch64 kernels omit "model name"; identify the core from implementer/part.
	var impl, part string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "CPU implementer"):
			impl = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		case strings.HasPrefix(line, "CPU part"):
			part = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	return armCoreName(impl, part)
}

// armCoreName maps ARM implementer/part idents to a human core name.
func armCoreName(impl, part string) string {
	if impl != "0x41" { // Arm Ltd
		return ""
	}
	switch part {
	case "0xd03":
		return "Cortex-A53"
	case "0xd04":
		return "Cortex-A35"
	case "0xd07":
		return "Cortex-A57"
	case "0xd08":
		return "Cortex-A72"
	case "0xd0a":
		return "Cortex-A73"
	case "0xd0b":
		return "Cortex-A76"
	case "0xd0c":
		return "Neoverse-N1"
	case "0xd0d":
		return "Cortex-A77"
	case "0xd0e":
		return "Cortex-A76AE"
	case "0xd41":
		return "Cortex-A78"
	case "0xd44":
		return "Cortex-X1"
	case "0xd46":
		return "Cortex-A78AE"
	case "0xd49":
		return "Neoverse-N2"
	case "0xd4e":
		return "Cortex-A510"
	case "0xd4f":
		return "Cortex-A710"
	case "0xd8c":
		return "Neoverse-V2"
	}
	return ""
}

func readFileLine(path string) string { return util.ReadFirstLine(path) }
