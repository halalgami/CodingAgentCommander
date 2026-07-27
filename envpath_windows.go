//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ensureLoginPATH makes sure the tools Commander shells out to (tmux — provided
// by the psmux shim — claude, litellm) are reachable before anything execs.
//
// Unlike macOS launchd, a Windows GUI process inherits the full per-user PATH
// from the registry, so scoop's shim dir and ~\.local\bin are normally already
// present. This is a best-effort fallback: if `tmux` still isn't resolvable we
// append the common per-user tool directories rather than fail every launch.
func ensureLoginPATH() {
	// psmux is xterm-compatible; a TERM helps the attach client render cleanly,
	// matching the Unix build. Harmless when already set.
	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}

	if _, err := exec.LookPath("tmux"); err == nil {
		return // PATH already usable
	}

	home, _ := os.UserHomeDir()
	extra := []string{
		filepath.Join(home, "scoop", "shims"),                                    // scoop (psmux, tools)
		filepath.Join(home, ".local", "bin"),                                     // claude, pip --user scripts
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"), // winget shims
	}
	os.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+strings.Join(extra, string(os.PathListSeparator)))
}
