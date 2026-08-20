package collect

import (
	"encoding/json"
	"strings"
	"testing"
)

const fixtureLSBLK = `{
   "blockdevices": [
      {
         "name": "mmcblk0",
         "type": "disk",
         "fstype": null,
         "mountpoint": null,
         "size": 31914983424,
         "ro": false,
         "children": [
            {
               "name": "mmcblk0p1",
               "type": "part",
               "fstype": "vfat",
               "mountpoint": "/boot",
               "size": 268435456,
               "ro": false
            },
            {
               "name": "mmcblk0p2",
               "type": "part",
               "fstype": "ext4",
               "mountpoint": "/",
               "size": 31566391296,
               "ro": false
            },
            {
               "name": "mmcblk0p3",
               "type": "part",
               "fstype": "swap",
               "mountpoint": "[SWAP]",
               "size": 1073741824,
               "ro": false
            }
         ]
      }
   ]
}`

// TestLsblkParse verifies the JSON shape that lsblk emits parses cleanly.
func TestLsblkParse(t *testing.T) {
	var parsed lsblkOutput
	if err := json.Unmarshal([]byte(fixtureLSBLK), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Blockdevices) != 1 {
		t.Fatalf("want 1 disk, got %d", len(parsed.Blockdevices))
	}
	disk := parsed.Blockdevices[0]
	if disk.Type != "disk" {
		t.Fatalf("want disk type, got %q", disk.Type)
	}
	if len(disk.Children) != 3 {
		t.Fatalf("want 3 partitions, got %d", len(disk.Children))
	}
	p2 := disk.Children[1]
	if p2.Name != "mmcblk0p2" || p2.Mountpoint == nil || *p2.Mountpoint != "/" {
		t.Fatalf("unexpected p2: %+v", p2)
	}
}

// TestBuildPartition verifies the merge of host mount metadata over lsblk data,
// including the [SWAP] skip and host-mount override.
func TestBuildPartition(t *testing.T) {
	c := &FilesystemCollector{Host: Host{Proc: "/proc", Root: ""}}
	mountByDev := map[string]struct {
		mount string
		fs    string
		ro    bool
	}{
		// Host table knows the true mountpoints (container view may differ).
		"mmcblk0p1": {mount: "/boot", fs: "vfat", ro: false},
		"mmcblk0p2": {mount: "/", fs: "ext4", ro: false},
	}
	swap := lsblkDevice{Name: "mmcblk0p3", Type: "part", FSType: "swap", Size: 1073741824}
	mount := "[SWAP]"
	swap.Mountpoint = &mount
	if _, ok := c.buildPartition(swap, mountByDev); ok {
		t.Fatal("swap pseudo-mount should be skipped")
	}

	mp := "/boot"
	p1 := lsblkDevice{Name: "mmcblk0p1", Type: "part", FSType: "vfat", Size: 268435456}
	p1.Mountpoint = &mp
	got, ok := c.buildPartition(p1, mountByDev)
	if !ok {
		t.Fatal("boot partition should be included")
	}
	if got.Mountpoint != "/boot" || got.Device != "/dev/mmcblk0p1" || got.Size != 268435456 {
		t.Fatalf("unexpected partition: %+v", got)
	}
}

// TestResolveDevName checks that /dev/root resolves to the real block name
// when a host root bind is present.
func TestResolveDevName(t *testing.T) {
	c := &FilesystemCollector{Host: Host{Proc: "/proc", Root: "/host/root"}}
	if got := c.resolveDevName("/dev/mmcblk0p1"); got != "mmcblk0p1" {
		t.Fatalf("plain device: want mmcblk0p1, got %q", got)
	}
	if got := c.resolveDevName("/dev/root"); got == "" || strings.Contains(got, " ") {
		t.Fatalf("unexpected /dev/root resolution: %q", got)
	}
}
