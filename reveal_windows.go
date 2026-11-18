//go:build windows

package main

import (
	"os/exec"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// revealDir opens dir in File Explorer. explorer.exe exits 1 even when it
// succeeds, so its exit code is ignored; only a failure to start counts.
var revealDir = func(dir string) error {
	return proc.Hide(exec.Command("explorer.exe", dir)).Start()
}
