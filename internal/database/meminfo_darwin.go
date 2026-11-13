//go:build darwin

package database

import "golang.org/x/sys/unix"

// platformTotalMemoryBytes reads the machine's physical memory. macOS has no
// /proc/meminfo, so POSTGRES_TUNE=auto would otherwise fail to size anything.
func platformTotalMemoryBytes() (int64, string, bool) {
	mem, err := unix.SysctlUint64("hw.memsize")
	if err != nil || mem == 0 || mem > 1<<62 {
		return 0, "", false
	}
	return int64(mem), "hw.memsize", true
}
