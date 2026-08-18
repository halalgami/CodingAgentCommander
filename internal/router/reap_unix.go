//go:build !windows

package router

import "os/exec"

// reapStale kills the orphan with pkill. -f matches against the whole command
// line rather than just the process name, which is what lets the generated
// config path identify one specific litellm invocation.
func reapStale(configPath string) {
	_ = exec.Command("pkill", "-f", configPath).Run()
}
