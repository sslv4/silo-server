package nodemetrics

import "context"

// samplePlatform takes one reading on a host sampled through platform APIs
// rather than /proc. Disks come from the same statfs reader Linux uses; CPU,
// memory and load come from the platform helpers. Cgroup and network counters
// have no equivalent and stay absent rather than reporting zero.
func (s *Sampler) samplePlatform(ctx context.Context) *Snapshot {
	now := s.now()
	pct, cores, busy, total, cpuOK := platformCPU(s.prevPlatformBusy, s.prevPlatformTotal)
	if cpuOK {
		s.prevPlatformBusy = busy
		s.prevPlatformTotal = total
	}

	stats := &SystemStats{Cores: cores}
	if cpuOK {
		stats.CPUPct = pct
	}
	if used, totalBytes, ok := platformMemory(); ok {
		stats.MemUsedMB = int64(used / (1 << 20))
		stats.MemTotalMB = int64(totalBytes / (1 << 20))
	}
	if load, ok := platformLoad1(); ok {
		stats.Load1 = load
	}

	paths := s.diskPaths(ctx)
	stats.Disks = s.diskStats(paths, now)

	return &Snapshot{Available: true, SampledAt: now, System: stats}
}
