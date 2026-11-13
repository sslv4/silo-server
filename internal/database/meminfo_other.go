//go:build !darwin

package database

// platformTotalMemoryBytes has no platform source outside darwin; detection
// falls through to the cgroup and /proc/meminfo readers.
func platformTotalMemoryBytes() (int64, string, bool) { return 0, "", false }
