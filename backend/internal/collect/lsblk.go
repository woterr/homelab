package collect

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Partition is a block partition with its mount and size, derived from lsblk
// on the host. It powers the "mounted drives" view.
type Partition struct {
	Device     string  `json:"device"`
	Name       string  `json:"name"`
	Size       uint64  `json:"size"`
	Mountpoint string  `json:"mountpoint"`
	FSType     string  `json:"fs_type"`
	ReadOnly   bool    `json:"read_only"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	UsedPct    float64 `json:"used_pct"`
}

type lsblkDevice struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	FSType     string        `json:"fstype"`
	Mountpoint *string       `json:"mountpoint"`
	Size       uint64        `json:"size"`
	RO         bool          `json:"ro"`
	Children   []lsblkDevice `json:"children"`
}

type lsblkOutput struct {
	Blockdevices []lsblkDevice `json:"blockdevices"`
}

// partitionsFromLsblk runs `lsblk -J -b` and flattens the device tree into
// mounted partitions. Mountpoints are taken from the host mount table rather
// than lsblk's own output because the container cannot see host mounts; usage
// is resolved via statfs through the host root bind when available.
func (c *FilesystemCollector) partitionsFromLsblk() []Partition {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b",
		"-o", "NAME,TYPE,FSTYPE,MOUNTPOINT,SIZE,RO")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var parsed lsblkOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil
	}

	// Host mount table: device -> mountpoint -> fstype/ro.
	mountByDev := map[string]struct {
		mount string
		fs    string
		ro    bool
	}{}
	for _, m := range c.parseMounts() {
		dev := c.resolveDevName(m.device)
		mountByDev[dev] = struct {
			mount string
			fs    string
			ro    bool
		}{mount: m.mount, fs: m.fstype, ro: m.readOnly}
	}

	var parts []Partition
	var walk func(ds []lsblkDevice)
	walk = func(ds []lsblkDevice) {
		for _, d := range ds {
			if d.Type == "disk" {
				// Whole disk mounted directly (rare) — treat as partition.
				if d.Mountpoint != nil && *d.Mountpoint != "" {
					if p, ok := c.buildPartition(d, mountByDev); ok {
						parts = append(parts, p)
					}
				}
				if len(d.Children) > 0 {
					walk(d.Children)
				}
				continue
			}
			// type part / lvm / crypt / md — include if mounted
			if d.Mountpoint != nil && *d.Mountpoint != "" {
				if p, ok := c.buildPartition(d, mountByDev); ok {
					parts = append(parts, p)
				}
			}
		}
	}
	walk(parsed.Blockdevices)
	return parts
}

// resolveDevName maps a host device path like /dev/root or /dev/sda1 to the
// stable block name lsblk reports. Symlinks such as /dev/root are resolved
// through the host-root bind so they join against the real partition.
func (c *FilesystemCollector) resolveDevName(dev string) string {
	name := strings.TrimPrefix(dev, "/dev/")
	if c.Host.Root == "" || !strings.HasPrefix(dev, "/dev/") {
		return name
	}
	if target, err := filepath.EvalSymlinks(filepath.Join(c.Host.Root, dev)); err == nil {
		seg := strings.TrimPrefix(target, c.Host.Root)
		seg = strings.TrimPrefix(seg, "/dev/")
		if seg != "" {
			name = seg
		}
	}
	return name
}

func (c *FilesystemCollector) buildPartition(d lsblkDevice, mountByDev map[string]struct {
	mount string
	fs    string
	ro    bool
}) (Partition, bool) {
	mountpoint := ""
	if d.Mountpoint != nil {
		mountpoint = *d.Mountpoint
	}
	fsType := d.FSType
	readOnly := d.RO
	// Prefer the host mount table: it is authoritative in containers and may
	// disagree with lsblk's in-container view.
	if m, ok := mountByDev[d.Name]; ok && m.mount != "" {
		mountpoint = m.mount
		if m.fs != "" {
			fsType = m.fs
		}
		readOnly = m.ro
	}
	// Skip pseudo-mounts such as [SWAP].
	if mountpoint == "" || strings.HasPrefix(mountpoint, "[") {
		return Partition{}, false
	}
	p := Partition{
		Device:     "/dev/" + d.Name,
		Name:       d.Name,
		Size:       d.Size,
		Mountpoint: mountpoint,
		FSType:     fsType,
		ReadOnly:   readOnly,
	}
	c.usageFor(&p)
	return p, true
}

// usageFor fills used/available from statfs, resolving the mountpoint through
// the host root bind so host-only drives report real numbers.
func (c *FilesystemCollector) usageFor(p *Partition) {
	st, err := statfsPath(c.statfsPath(p.Mountpoint))
	if err != nil {
		return
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return
	}
	avail := st.Bavail * uint64(st.Bsize)
	used := total - avail
	p.Used = used
	p.Available = avail
	if p.Size == 0 {
		p.Size = total
	}
	if total > 0 {
		p.UsedPct = float64(used) / float64(total) * 100
	}
}

func statfsPath(path string) (syscall.Statfs_t, error) {
	var st syscall.Statfs_t
	err := syscall.Statfs(path, &st)
	return st, err
}
