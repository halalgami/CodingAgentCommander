//go:build windows

package delegate

import "os/exec"

// setPgid is a no-op on Windows: SysProcAttr has no process-group equivalent.
// Killing a tree there needs a job object or a CIM walk; internal/router's
// reap_windows.go is the precedent if this ever matters for workers.
func setPgid(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
