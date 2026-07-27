//go:build windows

// Package proc centralizes OS-specific tweaks to child processes Commander
// spawns.
package proc

import (
	"os/exec"
	"syscall"
)

// Hide configures cmd so that launching a console program does not flash a
// console window on screen. Commander is a GUI app with no console of its own,
// so every child console process (the psmux `tmux` shim, litellm) would
// otherwise pop a brief window — glaringly visible on the frequent
// poll/reconcile calls (has-session, list-windows, display-message).
// CREATE_NO_WINDOW stops the console from being created at all.
func Hide(cmd *exec.Cmd) *exec.Cmd {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
	return cmd
}
