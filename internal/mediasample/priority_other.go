//go:build !linux && !darwin

package mediasample

import "os/exec"

// startBackground starts cmd at normal priority. Only Linux and macOS lower
// the priority of background runs.
func startBackground(cmd *exec.Cmd) error {
	return cmd.Start()
}
