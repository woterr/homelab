package collect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homelab/backend/internal/metric"
	"homelab/backend/internal/util"
)

// ThermalZone is one discovered temperature sensor.
type ThermalZone struct {
	Name     string  `json:"name"`
	Path     string  `json:"-"`
	TempC    float64 `json:"temp_c"`
	Critical float64 `json:"critical_c,omitempty"`
}

// TemperatureCollector discovers thermal zones and hwmon sensors. Where
// vcgencmd exists (Raspberry Pi) its extra measurements are included.
type TemperatureCollector struct {
	Host Host
}

func NewTemperatureCollector(h Host) *TemperatureCollector { return &TemperatureCollector{Host: h} }

func (c *TemperatureCollector) Name() string { return "temperature" }

// Zones discovers thermal_zone* and hwmon temp sensors.
func (c *TemperatureCollector) Zones() []ThermalZone {
	var out []ThermalZone
	base := c.Host.Sys + "/class/thermal"
	entries, err := os.ReadDir(base)
	if err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "thermal_zone") {
				continue
			}
			dir := filepath.Join(base, e.Name())
			t := util.ParseFloatOr(readFileLine(dir+"/temp"), -1)
			if t < 0 {
				continue
			}
			z := ThermalZone{
				Name:  util.ReadFirstLine(dir + "/type"),
				Path:  dir,
				TempC: t / 1000,
			}
			if crit := util.ParseFloatOr(readFileLine(dir+"/trip_point_0_temp"), -1); crit > 0 {
				z.Critical = crit / 1000
			}
			out = append(out, z)
		}
	}
	// hwmon sensors (may overlap with thermal zones; dedupe by temp value+name).
	hw, _ := os.ReadDir(c.Host.Sys + "/class/hwmon")
	for _, e := range hw {
		dir := c.Host.Sys + "/class/hwmon/" + e.Name()
		nm := util.ReadFirstLine(dir + "/name")
		inputs, _ := filepath.Glob(dir + "/temp*_input")
		for _, inp := range inputs {
			v := util.ParseFloatOr(util.ReadFirstLine(inp), -1)
			if v < 0 {
				continue
			}
			label := util.ReadFirstLine(strings.TrimSuffix(inp, "_input") + "_label")
			name := label
			if name == "" {
				name = nm
			}
			// skip duplicates already captured via thermal zones
			dup := false
			for _, z := range out {
				if z.Name == name && z.TempC == v/1000 {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			out = append(out, ThermalZone{Name: name, Path: inp, TempC: v / 1000})
		}
	}
	return out
}

func (c *TemperatureCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	now := time.Now()
	var out []metric.Sample
	for _, z := range c.Zones() {
		out = append(out, metric.NewL("temperature", map[string]string{"sensor": z.Name}, now, z.TempC))
	}
	return out, nil
}

// Snapshot returns the current sensor readings.
func (c *TemperatureCollector) Snapshot() []ThermalZone { return c.Zones() }

// Temperature satisfies TemperatureSource.
func (c *TemperatureCollector) Temperature() []ThermalZone { return c.Zones() }
