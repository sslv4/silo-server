//go:build darwin

package embeddedsub

import (
	"os"

	"golang.org/x/sys/unix"
)

// adviseRandom turns read-ahead off for f. macOS has no posix_fadvise;
// F_RDAHEAD 0 is the equivalent hint for a random access pattern. It is
// advice: a failure only costs read-ahead.
func adviseRandom(f *os.File) {
	conn, err := f.SyscallConn()
	if err != nil {
		return
	}
	_ = conn.Control(func(fd uintptr) {
		_, _ = unix.FcntlInt(fd, unix.F_RDAHEAD, 0)
	})
}
