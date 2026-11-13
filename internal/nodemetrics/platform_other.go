//go:build !darwin

package nodemetrics

// platformSamplingSupported reports whether this build can sample the host
// through a platform API. Linux reads /proc and cgroup files directly, so it
// does not use these helpers.
const platformSamplingSupported = false

func platformCPU(uint64, uint64) (int, int, uint64, uint64, bool) { return 0, 0, 0, 0, false }
func platformMemory() (uint64, uint64, bool)                      { return 0, 0, false }
func platformLoad1() (float64, bool)                              { return 0, false }
