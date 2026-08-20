package collect

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"homelab/backend/internal/metric"
)

// FilesystemUsage is one mounted filesystem with capacity/usage in bytes.
type FilesystemUsage struct {
	Mountpoint string  `json:"mountpoint"`
	Device     string  `json:"device"`
	FSType     string  `json:"fs_type"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	UsedPct    float64 `json:"used_pct"`
	ReadOnly   bool    `json:"read_only"`
}

// FilesystemCollector reports capacity/usage for physical filesystems.
type FilesystemCollector struct {
	Host Host
}

func NewFilesystemCollector(h Host) *FilesystemCollector { return &FilesystemCollector{Host: h} }

func (c *FilesystemCollector) Name() string { return "filesystem" }

// Read parses /proc/mounts and stats each mount with statfs. Host mountpoints
// are resolved through the container's bind-mounted host root so mounted
// drives (USB, SATA, ...) that are absent from the container namespace still
// report real capacity and usage.
func (c *FilesystemCollector) Read() []FilesystemUsage {
	mounts := c.parseMounts()
	var out []FilesystemUsage
	for _, m := range mounts {
		var st syscall.Statfs_t
		path := c.statfsPath(m.mount)
		if err := syscall.Statfs(path, &st); err != nil {
			// Fall back to the raw path (e.g. the container's own volume).
			if path == m.mount || syscall.Statfs(m.mount, &st) != nil {
				continue
			}
		}
		// Skip pseudo filesystems with zero blocks.
		if st.Blocks == 0 {
			continue
		}
		total := st.Blocks * uint64(st.Bsize)
		avail := st.Bavail * uint64(st.Bsize)
		used := total - avail
		var pct float64
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		out = append(out, FilesystemUsage{
			Mountpoint: m.mount,
			Device:     m.device,
			FSType:     m.fstype,
			Total:      total,
			Used:       used,
			Available:  avail,
			UsedPct:    pct,
			ReadOnly:   m.readOnly,
		})
	}
	return out
}

// statfsPath maps a host mountpoint to a path reachable from the container. If
// Host.Root is set (host root bind-mounted read-only) every host path can be
// resolved beneath it, so host-only mounts like /mnt/sdcard are observable.
func (c *FilesystemCollector) statfsPath(mount string) string {
	if c.Host.Root == "" {
		return mount
	}
	return filepath.Join(c.Host.Root, mount)
}

type mount struct {
	device, mount, fstype string
	readOnly              bool
}

func (c *FilesystemCollector) parseMounts() []mount {
	f, err := os.Open(c.Host.Proc + "/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		m := mount{device: fields[0], mount: fields[1], fstype: fields[2]}
		if len(fields) > 3 {
			m.readOnly = strings.Contains(fields[3], "ro")
		}
		if strings.HasPrefix(m.fstype, "fuse") || m.fstype == "tmpfs" || m.fstype == "devtmpfs" ||
			m.fstype == "proc" || m.fstype == "sysfs" || m.fstype == "cgroup" || m.fstype == "overlay" ||
			m.fstype == "autofs" || m.fstype == "nsfs" || m.fstype == "pstore" || m.fstype == "securityfs" ||
			m.fstype == "debugfs" || m.fstype == "tracefs" || m.fstype == "mqueue" || m.fstype == "bpf" ||
			m.fstype == "configfs" || m.fstype == "binfmt_misc" || m.fstype == "devpts" ||
			m.fstype == "ramfs" || m.fstype == "zram" || m.fstype == "squashfs" ||
			m.fstype == "erofs" || m.fstype == "iso9660" {
			continue
		}
		// Skip bind-mounted proc/sys/dev that appear with a normal fstype.
		if strings.HasPrefix(m.mount, "/proc/") || strings.HasPrefix(m.mount, "/sys/") ||
			strings.HasPrefix(m.mount, "/dev/") || strings.HasPrefix(m.mount, "/host/") {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (c *FilesystemCollector) Collect(ctx context.Context) ([]metric.Sample, error) {
	now := time.Now()
	var out []metric.Sample
	for _, fs := range c.Read() {
		labels := map[string]string{"mount": fs.Mountpoint, "device": fs.Device, "fstype": fs.FSType}
		out = append(out,
			metric.NewL("filesystem.total_bytes", labels, now, float64(fs.Total)),
			metric.NewL("filesystem.used_bytes", labels, now, float64(fs.Used)),
			metric.NewL("filesystem.available_bytes", labels, now, float64(fs.Available)),
			metric.NewL("filesystem.used_percent", labels, now, fs.UsedPct),
		)
	}
	return out, nil
}

// Filesystems satisfies FilesystemSource.
func (c *FilesystemCollector) Filesystems() []FilesystemUsage { return c.Read() }

// Partitions satisfies PartitionSource: it enumerates mounted block partitions
// with lsblk (the authoritative device listing on the host) and derives usage
// from statfs, resolving mountpoints through the host root bind.
func (c *FilesystemCollector) Partitions() []Partition {
	if c.Host.Proc == "" {
		return nil
	}
	return c.partitionsFromLsblk()
}
