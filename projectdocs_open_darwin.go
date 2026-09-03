//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// openInDefaultApp hands path to LaunchServices. The argument goes in an argv
// slice with no shell, and "--" stops a filename beginning with a dash from
// being read as a flag.
func openInDefaultApp(path string) error {
	if err := exec.Command("open", "--", path).Run(); err != nil {
		return fmt.Errorf("that file could not be opened externally: %w", err)
	}
	return nil
}
