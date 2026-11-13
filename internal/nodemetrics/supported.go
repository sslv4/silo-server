package nodemetrics

import "runtime"

// SamplingSupported reports whether this host can be sampled: Linux reads
// /proc and cgroup files, and platforms with a native API report through it.
// Admin surfaces use this instead of testing the OS name, so adding a platform
// does not need a second change here.
func SamplingSupported() bool {
	return runtime.GOOS == "linux" || platformSamplingSupported
}
