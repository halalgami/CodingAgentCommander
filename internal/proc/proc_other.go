//go:build !windows

// Package proc centralizes OS-specific tweaks to child processes Commander
// spawns.
package proc

import "os/exec"

// Hide is a no-op off Windows: console child processes there don't spawn
// visible windows, so there is nothing to suppress.
func Hide(cmd *exec.Cmd) *exec.Cmd { return cmd }
