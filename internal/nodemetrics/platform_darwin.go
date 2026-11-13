//go:build darwin

package nodemetrics

import (
	"encoding/binary"
	"runtime"

	"golang.org/x/sys/unix"
)

// platformSamplingSupported reports whether this build can sample the host.
const platformSamplingSupported = true

// platformCPU reports busy percentage since prev and the CPU count. macOS has
// no cgroup quota, so cores is always every CPU the kernel reports.
func platformCPU(prevBusy, prevTotal uint64) (pct, cores int, busy, total uint64, ok bool) {
	cores = runtime.NumCPU()
	busy, total, ok = darwinCPUTicks()
	if !ok {
		return 0, cores, 0, 0, false
	}
	// The first pass has no previous reading to subtract.
	if prevTotal == 0 || total <= prevTotal {
		return 0, cores, busy, total, true
	}
	deltaBusy := busy - prevBusy
	deltaTotal := total - prevTotal
	if deltaTotal == 0 {
		return 0, cores, busy, total, true
	}
	pct = int((deltaBusy * 100) / deltaTotal)
	if pct > 100 {
		pct = 100
	}
	return pct, cores, busy, total, true
}

// platformMemory reports used and total bytes.
func platformMemory() (used, total uint64, ok bool) {
	t, err := unix.SysctlUint64("hw.memsize")
	if err != nil || t == 0 {
		return 0, 0, false
	}
	u, ok := darwinMemoryUsedBytes()
	if !ok {
		return 0, t, false
	}
	return u, t, true
}

// platformLoad1 reports the 1-minute load average. macOS exposes it as a
// binary struct loadavg { fixpt_t ldavg[3]; long fscale; } through sysctl,
// not as the text /proc/loadavg holds, so the raw bytes are decoded here.
func platformLoad1() (float64, bool) {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 12 {
		return 0, false
	}
	avg := binary.LittleEndian.Uint32(raw[0:4])
	// fscale follows the three averages; it is a long, so its offset depends
	// on the pointer size. 64-bit darwin pads the array to an 8-byte boundary.
	var scale uint64
	switch {
	case len(raw) >= 24:
		scale = binary.LittleEndian.Uint64(raw[16:24])
	case len(raw) >= 16:
		scale = uint64(binary.LittleEndian.Uint32(raw[12:16]))
	}
	if scale == 0 {
		return 0, false
	}
	return float64(avg) / float64(scale), true
}
