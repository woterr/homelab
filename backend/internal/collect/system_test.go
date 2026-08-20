package collect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSOCName(t *testing.T) {
	dir := t.TempDir()
	dt := filepath.Join(dir, "device-tree")
	if err := os.MkdirAll(dt, 0o755); err != nil {
		t.Fatal(err)
	}
	// compatible entries are NUL-separated; Pi 4 firmware reports brcm,bcm2711.
	compatible := "raspberrypi,4-model-b\x00brcm,bcm2711\x00"
	if err := os.WriteFile(filepath.Join(dt, "compatible"), []byte(compatible), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := socName(dir); got != "BCM2711" {
		t.Fatalf("socName = %q, want BCM2711", got)
	}

	// Pi 3 reports brcm,bcm2837.
	compatible = "raspberrypi,3-model-b\x00brcm,bcm2837\x00"
	if err := os.WriteFile(filepath.Join(dt, "compatible"), []byte(compatible), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := socName(dir); got != "BCM2837" {
		t.Fatalf("socName = %q, want BCM2837", got)
	}

	// No broadcom entry -> empty.
	if err := os.WriteFile(filepath.Join(dt, "compatible"), []byte("intel,generic\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := socName(dir); got != "" {
		t.Fatalf("socName = %q, want empty", got)
	}
}

func TestArmCoreName(t *testing.T) {
	// Pi 4: implementer 0x41 (Arm), part 0xd08 (Cortex-A72).
	if got := armCoreName("0x41", "0xd08"); got != "Cortex-A72" {
		t.Fatalf("armCoreName = %q, want Cortex-A72", got)
	}
	if got := armCoreName("0x41", "0xd03"); got != "Cortex-A53" {
		t.Fatalf("armCoreName = %q, want Cortex-A53", got)
	}
	if got := armCoreName("0x42", "0xd08"); got != "" {
		t.Fatalf("armCoreName = %q for non-Arm implementer, want empty", got)
	}
	if got := armCoreName("0x41", "0x000"); got != "" {
		t.Fatalf("armCoreName = %q for unknown part, want empty", got)
	}
}