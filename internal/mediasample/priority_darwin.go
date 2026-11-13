//go:build darwin

package mediasample

import (
	"log/slog"
	"os/exec"
	"sync"

	"golang.org/x/sys/unix"
)

// backgroundNice is the lowest CPU priority, matching the Linux path.
const backgroundNice = 19

var priorityWarning sync.Once

// startBackground starts cmd and lowers the child's CPU priority.
//
// macOS keeps nice per process rather than per thread, so the Linux approach
// of lowering a locked thread before forking would lower the whole server.
// The child is renice'd after it starts instead: it runs at normal priority
// only for the moment between, and analysis still yields to playback for the
// rest of its life.
func startBackground(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process == nil {
		return nil
	}
	if err := unix.Setpriority(unix.PRIO_PROCESS, cmd.Process.Pid, backgroundNice); err != nil {
		priorityWarning.Do(func() {
			slog.Warn("could not lower background media sampling priority", "error", err)
		})
	}
	return nil
}
