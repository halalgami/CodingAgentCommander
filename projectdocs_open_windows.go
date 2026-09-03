//go:build windows

package main

import (
	"fmt"
	"os/exec"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// openInDefaultApp uses url.dll's FileProtocolHandler rather than
// `cmd /c start`: cmd parses its own arguments and has a quoting history not
// worth inheriting. proc.Hide keeps a console window from flashing, as every
// other child process in this app does.
func openInDefaultApp(path string) error {
	cmd := proc.Hide(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("that file could not be opened externally: %w", err)
	}
	return nil
}
