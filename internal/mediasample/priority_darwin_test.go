//go:build darwin

package mediasample

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

// startBackground must lower the child's priority without touching the
// server's own: macOS nice is per process, so a careless implementation would
// renice Silo itself.
func TestStartBackgroundLowersChildNotSelf(t *testing.T) {
	before, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
	if err != nil {
		t.Fatalf("Getpriority() error = %v", err)
	}

	cmd := exec.Command("/bin/sleep", "5")
	if err := startBackground(cmd); err != nil {
		t.Fatalf("startBackground() error = %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// unix.Getpriority returns the priority biased so that it is never
	// negative; compare like with like rather than against the raw nice value.
	child, err := unix.Getpriority(unix.PRIO_PROCESS, cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpriority(child) error = %v", err)
	}
	if child <= before {
		t.Errorf("child priority = %d, own = %d; want the child lowered", child, before)
	}

	after, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
	if err != nil {
		t.Fatalf("Getpriority(self) error = %v", err)
	}
	if after != before {
		t.Errorf("own priority changed from %d to %d; background runs must not renice the server", before, after)
	}
}

// A command that cannot start must report the error rather than panicking on
// a nil Process.
func TestStartBackgroundReportsStartFailure(t *testing.T) {
	if err := startBackground(exec.Command("/nonexistent/silo-test-binary")); err == nil {
		t.Fatal("startBackground() error = nil, want a start failure")
	}
}
