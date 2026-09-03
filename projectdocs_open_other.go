//go:build !darwin && !windows

package main

import (
	"fmt"
	"os/exec"
)

// openInDefaultApp uses the freedesktop opener. Absent (a bare container, a
// minimal WM) it fails, and the viewer reports that rather than pretending.
func openInDefaultApp(path string) error {
	if err := exec.Command("xdg-open", path).Run(); err != nil {
		return fmt.Errorf("that file could not be opened externally: %w", err)
	}
	return nil
}
