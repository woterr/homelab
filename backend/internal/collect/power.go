package collect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homelab/backend/internal/metric"
)

// PowerSample carries power-related measurements when the hardware exposes
// them (e.g. Raspberry Pi). Fields are zero when unavailable so the UI can
// show honest "not available" states.
type PowerSample struct {
	Available  bool    `json:"available"`
	VoltageV   float64 `json:"voltage_v,omitempty"`
	Throttled  string  `json:"throttled,omitempty"`
	PowerState string  `json:"power_state"`
}

// PowerCollector reads power telemetry via vcgencmd when it is reachable
// (running on the host) and otherwise falls back to sysfs entries that are
// visible through a bind-mounted /sys (/host/sys). On non-Raspberry-Pi
// hardware it reports unavailable without fabricating values.
type PowerCollector struct {
	Host Host
}

func NewPowerCollector(h Host) *PowerCollector { return &PowerCollector{Host: h} }

func (c *PowerCollector) Name() string { return "power" }

func (c *PowerCollector) read() PowerSample {
	ps := PowerSample{PowerState: "unavailable"}

	// vcgencmd only works on the host itself; when the daemon runs inside a
	// container it is usually absent, so sysfs is the primary source there.
	throttled := c.sysfsThrottled()
	if t := runVcgencmd("get_throttled"); t != "" {
		throttled = strings.TrimSpace(t)
	}
	if throttled != "" {
		ps.Throttled = throttled
		ps.Available = true
		ps.PowerState = "nominal"
	}

	voltage := c.sysfsVoltageV()
	if v := runVcgencmd("measure_volts"); v != "" {
		v = strings.TrimSuffix(strings.TrimSpace(v), "V")
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			voltage = f
		}
	}
	if voltage > 0 {
		ps.VoltageV = voltage
		ps.Available = true
		ps.PowerState = "nominal"
	}
	return ps
}

// sysfsThrottled reads the Raspberry Pi firmware throttling bitmask from sysfs
// (kernel >= 5.x exposes it at soc:firmware/get_throttled).
func (c *PowerCollector) sysfsThrottled() string {
	paths := []string{
		c.Host.Sys + "/devices/platform/soc/soc:firmware/get_throttled",
		c.Host.Sys + "/class/raspberrypi/firmware_throttled",
	}
	for _, p := range paths {
		v := readFileLine(p)
		if v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// sysfsVoltageV scans hwmon inputs for the Raspberry Pi core voltage, which is
// reported in millivolts by the raspberrypi-hwmon driver. Only inputs that
// clearly belong to the Pi's power rail are accepted so unrelated hwmon
// sensors on other hardware are not mistaken for core voltage.
func (c *PowerCollector) sysfsVoltageV() float64 {
	var dirs []string
	classHW := c.Host.Sys + "/class/hwmon"
	if entries, err := os.ReadDir(classHW); err == nil {
		for _, e := range entries {
			dirs = append(dirs, filepath.Join(classHW, e.Name()))
		}
	}
	// Direct path on Pi 5 / newer firmware where the driver lives under the
	// firmware platform device.
	fwDirs := []string{c.Host.Sys + "/devices/platform/soc/soc:firmware/hwmon"}
	for _, d := range fwDirs {
		if entries, err := os.ReadDir(d); err == nil {
			for _, e := range entries {
				dirs = append(dirs, filepath.Join(d, e.Name()))
			}
		}
	}
	for _, dir := range dirs {
		name := strings.ToLower(readFileLine(dir + "/name"))
		if strings.Contains(name, "raspberrypi") || strings.Contains(name, "bcm") {
			// The Pi's own hwmon reports core voltage unconditionally.
			if mv := utilParseFloat(readFileLine(dir + "/in0_input")); mv > 0 {
				return mv / 1000
			}
		}
		label := strings.ToLower(readFileLine(dir + "/in0_label"))
		if strings.Contains(label, "core") || strings.Contains(label, "vcore") {
			if mv := utilParseFloat(readFileLine(dir + "/in0_input")); mv > 0 {
				return mv / 1000
			}
		}
	}
	return 0
}

func utilParseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func (c *PowerCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	ps := c.read()
	now := time.Now()
	if !ps.Available {
		return []metric.Sample{
			metric.New("power.available", now, 0),
		}, nil
	}
	out := []metric.Sample{
		metric.New("power.available", now, 1),
	}
	if ps.VoltageV > 0 {
		out = append(out, metric.New("power.voltage_v", now, ps.VoltageV))
	}
	return out, nil
}

// Snapshot returns the latest power measurement.
func (c *PowerCollector) Snapshot() PowerSample { return c.read() }

// Power satisfies PowerSource.
func (c *PowerCollector) Power() PowerSample { return c.read() }

func runVcgencmd(args ...string) string {
	cmd := exec.Command("vcgencmd", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
