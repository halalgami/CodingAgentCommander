//go:build windows

package deps

// underManagedDir and probeVersion are shared with the Unix build; see deps.go.

// Status snapshots every external dependency for the frontend, in the order the
// UI should present them: the session host first, then the shell it runs panes
// in, then the agent itself.
func Status() []Tool {
	return []Tool{tmuxTool(), pwshTool(), claudeTool()}
}

// tmuxTool describes the session host. On Windows that is psmux, which installs
// a `tmux` shim — the name Commander actually execs (see internal/tmux.tmuxCmd).
// A release built with the `bundled` tag carries its own copy, so Found is false
// here only when the bundled extraction itself failed.
func tmuxTool() Tool {
	t := Tool{
		Name:     "tmux",
		Label:    "tmux (psmux)",
		Required: true,
		Hint:     "winget install marlocarlo.psmux",
	}
	if p, ok := lookPath("tmux"); ok {
		t.Path, t.Found = p, true
		t.Managed = underManagedDir(p)
		t.Version = probeVersion(p, "-V")
	}
	return t
}

// pwshTool describes PowerShell 7. Commander does not exec pwsh itself; psmux
// does, for every pane it opens — its Claude Code integration requires 7+, and
// the Windows PowerShell 5.1 that ships with Windows does not qualify. So this
// is a required dependency that only ever fails at session time, which is
// exactly why it belongs in a startup status panel.
//
// No version probe: pwsh takes the better part of a second to start, and Status
// runs on every UI refresh. Presence on PATH under the name `pwsh` already
// implies 7+ — 5.1 is `powershell`.
func pwshTool() Tool {
	t := Tool{
		Name:       "pwsh",
		Label:      "PowerShell 7+",
		Required:   true,
		CanInstall: true,
		Hint:       "winget install Microsoft.PowerShell",
	}
	if p, ok := lookPath("pwsh"); ok {
		t.Path, t.Found = p, true
		t.Managed = underManagedDir(p)
		return t
	}
	// ensureLoginPATH adds the managed copy to PATH when it exists, so reaching
	// here normally means it is absent — but check directly too, so a Status
	// call from a context with an untouched PATH still reports it.
	if isExecFile(ManagedPwsh()) {
		t.Path, t.Found, t.Managed = ManagedPwsh(), true, true
	}
	return t
}
