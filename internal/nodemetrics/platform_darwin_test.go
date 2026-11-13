//go:build darwin

package nodemetrics

import (
	"context"
	"testing"
	"time"
)

// Without a platform reading the sampler stores an empty Snapshot, so Admin
// reports "unsupported" and every resource panel stays blank.
func TestPlatformMemoryReportsUsedAndTotal(t *testing.T) {
	used, total, ok := platformMemory()
	if !ok {
		t.Fatal("platformMemory() not ok on darwin")
	}
	if total == 0 {
		t.Fatal("total memory is 0")
	}
	if used == 0 || used >= total {
		t.Errorf("used = %d, total = %d; want 0 < used < total", used, total)
	}
}

func TestPlatformCPUReportsCoresAndBoundedPercent(t *testing.T) {
	// The first call has no previous reading, so it reports 0% and seeds the
	// counters; the second produces a real delta.
	_, cores, busy, total, ok := platformCPU(0, 0)
	if !ok {
		t.Fatal("platformCPU() not ok on darwin")
	}
	if cores <= 0 {
		t.Errorf("cores = %d, want positive", cores)
	}
	if total == 0 {
		t.Fatal("total ticks = 0")
	}

	time.Sleep(50 * time.Millisecond)
	pct, _, busy2, total2, ok := platformCPU(busy, total)
	if !ok {
		t.Fatal("second platformCPU() not ok")
	}
	if pct < 0 || pct > 100 {
		t.Errorf("cpu pct = %d, want 0..100", pct)
	}
	if total2 < total || busy2 < busy {
		t.Errorf("tick counters went backwards: busy %d->%d total %d->%d", busy, busy2, total, total2)
	}
}

// A first sample with no previous ticks must report 0%, not a percentage
// computed against a zero baseline.
func TestPlatformCPUFirstSampleReportsZero(t *testing.T) {
	pct, _, _, _, ok := platformCPU(0, 0)
	if !ok {
		t.Fatal("platformCPU() not ok")
	}
	if pct != 0 {
		t.Errorf("first-sample pct = %d, want 0", pct)
	}
}

func TestPlatformLoad1IsPlausible(t *testing.T) {
	load, ok := platformLoad1()
	if !ok {
		t.Fatal("platformLoad1() not ok on darwin")
	}
	if load < 0 || load > 1024 {
		t.Errorf("load1 = %v, want a plausible average", load)
	}
}

// SamplingSupported gates the Admin capability; if it disagrees with the
// sampler, Admin says unsupported while numbers are being collected.
func TestSamplingSupportedOnDarwin(t *testing.T) {
	if !SamplingSupported() {
		t.Error("SamplingSupported() = false on darwin, want true")
	}
}

// The platform path must publish a usable snapshot rather than the empty one
// the unsupported branch stores.
func TestSamplePlatformPublishesSystemStats(t *testing.T) {
	s := &Sampler{
		goos:         "darwin",
		now:          time.Now,
		scratchDirFn: func() string { return t.TempDir() },
	}
	snap := s.samplePlatform(context.Background())
	if snap == nil || !snap.Available {
		t.Fatal("samplePlatform() produced an unavailable snapshot")
	}
	if snap.System == nil {
		t.Fatal("snapshot has no System stats")
	}
	if snap.System.Cores <= 0 {
		t.Errorf("cores = %d, want positive", snap.System.Cores)
	}
	if snap.System.MemTotalMB <= 0 {
		t.Errorf("mem total = %d MB, want positive", snap.System.MemTotalMB)
	}
	if snap.SampledAt.IsZero() {
		t.Error("SampledAt is zero")
	}
}
