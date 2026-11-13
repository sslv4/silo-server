//go:build darwin

package database

import "testing"

// Without a platform source, POSTGRES_TUNE=auto cannot size anything on macOS:
// there is no /proc/meminfo to fall back to.
func TestPlatformTotalMemoryBytesReportsPhysicalMemory(t *testing.T) {
	mem, source, ok := platformTotalMemoryBytes()
	if !ok {
		t.Fatal("platformTotalMemoryBytes() reported no memory on darwin")
	}
	if source != "hw.memsize" {
		t.Errorf("source = %q, want hw.memsize", source)
	}
	// A Mac that can run Silo has more than 1 GiB and less than 4 TiB; the
	// bounds only catch a unit or sign error, not a specific machine.
	const minBytes = 1 << 30
	const maxBytes = 4 << 40
	if mem < minBytes || mem > maxBytes {
		t.Errorf("memory = %d bytes, want between %d and %d", mem, minBytes, maxBytes)
	}
}

// detectTotalMemoryBytes must reach the darwin source rather than failing on
// the missing /proc/meminfo.
func TestDetectTotalMemoryBytesSucceedsOnDarwin(t *testing.T) {
	mem, source, err := detectTotalMemoryBytes()
	if err != nil {
		t.Fatalf("detectTotalMemoryBytes() error = %v, want a reading", err)
	}
	if mem <= 0 {
		t.Errorf("memory = %d, want positive", mem)
	}
	if source == "" {
		t.Error("source is empty")
	}
}
