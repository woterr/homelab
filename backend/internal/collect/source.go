package collect

import (
	"context"

	"homelab/backend/internal/store"
)

// Source interfaces let the API read snapshot data uniformly from either real
// collectors or the deterministic demo host. Method names are unique per
// source so a single type (the demo host) can implement all of them.
type (
	SystemSource      interface{ Info() SystemInfo }
	CPUSource         interface{ CPU() *CPUSample }
	MemorySource      interface{ Memory() MemorySample }
	TemperatureSource interface{ Temperature() []ThermalZone }
	NetworkSource     interface {
		Network() ([]NetInterface, string)
	}
	DiskSource       interface{ Disk() []DiskStats }
	FilesystemSource interface{ Filesystems() []FilesystemUsage }
	PartitionSource  interface{ Partitions() []Partition }
	ServiceSource    interface {
		Services(ctx context.Context) ([]*store.Service, error)
	}
	DeviceSource interface{ Devices() []*store.Device }
	PowerSource  interface{ Power() PowerSample }
)
