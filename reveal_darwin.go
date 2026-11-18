//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// revealDir opens dir in Finder. "-a Finder" matters: plain `open` on a
// folder that is an app bundle would launch the app instead.
var revealDir = func(dir string) error {
	if err := exec.Command("open", "-a", "Finder", "--", dir).Run(); err != nil {
		return fmt.Errorf("Finder could not open that folder: %w", err)
	}
	return nil
}
