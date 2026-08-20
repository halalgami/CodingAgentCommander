//go:build !windows

package deps

// Status snapshots every external dependency for the frontend. pwsh is absent
// from this list by design: it is a Windows-only requirement, imposed by psmux's
// Claude Code integration rather than by Commander, and upstream tmux on macOS
// runs panes in the user's own shell.
func Status() []Tool {
	return []Tool{tmuxTool(), claudeTool()}
}

// tmuxTool describes the session host. Nothing is bundled on macOS: tmux comes
// from homebrew, the DMG is not a portable single file the way the Windows exe
// is, and `brew install tmux` is a one-liner every Mac developer already has a
// path for.
func tmuxTool() Tool {
	t := Tool{
		Name:     "tmux",
		Label:    "tmux",
		Required: true,
		Hint:     "brew install tmux",
	}
	if p, ok := lookPath("tmux"); ok {
		t.Path, t.Found = p, true
		t.Version = probeVersion(p, "-V")
	}
	return t
}
