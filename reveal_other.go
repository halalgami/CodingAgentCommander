//go:build !darwin && !windows

package main

import (
	"fmt"
	"os/exec"
)

// revealDir hands dir to the desktop's file manager.
var revealDir = func(dir string) error {
	if err := exec.Command("xdg-open", dir).Run(); err != nil {
		return fmt.Errorf("that folder could not be opened: %w", err)
	}
	return nil
}
