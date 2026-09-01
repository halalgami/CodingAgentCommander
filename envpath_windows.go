//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/deps"
)

// ensureLoginPATH makes sure the tools Commander shells out to (tmux — provided
// by the psmux shim — pwsh, claude, litellm) are reachable before anything
// execs.
//
// Unlike macOS launchd, a Windows GUI process inherits the full per-user PATH
// from the registry, so scoop's shim dir and ~\.local\bin are normally already
// present. The fallbacks below exist for the case that is not theoretical: a
// package manager can report a tool as installed while its entry point is
// missing or unregistered — a winget package whose Links shim was never
// recreated, a scoop install whose shims dir is off PATH — and the user then
// sees `exec: "tmux": executable file not found in %PATH%` while winget insists
// psmux is installed.
//
// For tmux, the release closes that hole outright by carrying its own psmux
// (see internal/deps): if nothing on PATH provides tmux, we extract and use the
// bundled copy rather than failing the launch.
func ensureLoginPATH() {
	// psmux is xterm-compatible; a TERM helps the attach client render cleanly,
	// matching the Unix build. Harmless when already set.
	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}

	var extra []string

	// tmux is the one dependency Commander cannot run without. Whatever the user
	// installed stays authoritative: the per-user tool dirs go on PATH first, and
	// the bundled copy only after them, so an explicit psmux still wins.
	if _, err := exec.LookPath("tmux"); err != nil {
		extra = append(extra, perUserToolDirs()...)
		if dir, err := deps.EnsureBundledPsmux(); err == nil {
			extra = append(extra, dir)
		}
	}

	// pwsh 7 — psmux runs it in every pane it opens, so a missing pwsh breaks
	// sessions rather than the app. Checked independently of tmux: the two are
	// installed by different package managers and go missing separately.
	if _, err := exec.LookPath("pwsh"); err != nil {
		extra = append(extra, perUserToolDirs()...)
		extra = append(extra, deps.ManagedPwshDir())
	}

	appendPATH(extra)
}

// perUserToolDirs lists the places Windows package managers drop per-user CLI
// entry points. Chocolatey's bin is included even though it is normally on the
// machine PATH — it costs one Stat, and "normally" is what this function exists
// to stop relying on.
func perUserToolDirs() []string {
	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	return []string{
		filepath.Join(home, "scoop", "shims"),                       // scoop (psmux, tools)
		filepath.Join(home, ".local", "bin"),                        // claude, pip --user scripts
		filepath.Join(local, "Microsoft", "WinGet", "Links"),        // winget shims
		filepath.Join(local, "Microsoft", "WindowsApps"),            // MSIX aliases (pwsh installs here)
		filepath.Join(local, "Programs", "PowerShell", "7"),         // pwsh, per-user MSI
		filepath.Join(os.Getenv("ProgramFiles"), "PowerShell", "7"), // pwsh, machine MSI
		filepath.Join(os.Getenv("ProgramData"), "chocolatey", "bin"),
	}
}

// appendPATH adds dirs to this process's PATH, skipping blanks, duplicates and
// paths that do not exist. Children inherit the result — the psmux server, and
// the claude processes inside its panes.
//
// Skipping non-existent directories is what makes the managed-pwsh entry
// self-managing: it is simply absent from PATH until the in-app install has
// created it, with no separate "is it installed yet" check to keep in sync.
func appendPATH(dirs []string) {
	cur := os.Getenv("PATH")
	seen := map[string]bool{}
	for _, p := range filepath.SplitList(cur) {
		seen[strings.ToLower(filepath.Clean(p))] = true
	}
	var add []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(d))
		if seen[key] {
			continue
		}
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			continue
		}
		seen[key] = true
		add = append(add, d)
	}
	if len(add) == 0 {
		return
	}
	sep := string(os.PathListSeparator)
	os.Setenv("PATH", cur+sep+strings.Join(add, sep))
}
