//go:build !windows

package delegate

import (
	"os/exec"
	"syscall"
)

// setPgid puts the worker in its own process group so killGroup can take its
// children with it. internal/proc.Hide does NOT do this — it is a documented
// no-op off Windows — so a plain Process.Kill leaves grandchildren running.
func setPgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup signals the whole group. The negative pid is the group.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
