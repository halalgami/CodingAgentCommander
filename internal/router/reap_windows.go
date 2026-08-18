//go:build windows

package router

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// reapConfigEnv carries the config path to the reap script out-of-band. Splicing
// a filesystem path into the script text would mean quoting arbitrary spaces and
// backslashes correctly, and would let a crafted path inject PowerShell. Passing
// it in the environment sidesteps both — and has a third benefit: the script's
// own command line then contains only the literal `$env:` reference, so the
// reaper cannot match and kill itself.
const reapConfigEnv = "COMMANDER_REAP_CONFIG"

// reapScript stops every process whose command line mentions the config path.
//
// Windows has no pkill, and taskkill cannot help: its /FI filters cover image
// name, PID, service and window title — never the command line — so an orphan
// can only be identified through a WMI/CIM process query.
//
// IndexOf with OrdinalIgnoreCase rather than -like: Windows paths are
// case-insensitive, and -like would additionally treat [, ] and * in the path as
// wildcard syntax. Stop-Process failures (the process died between the query and
// the kill, or belongs to another user) are ignored — this is best-effort.
const reapScript = `$p = $env:COMMANDER_REAP_CONFIG
if (-not $p) { exit 0 }
Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
  Where-Object { $_.CommandLine -and $_.CommandLine.IndexOf($p, [System.StringComparison]::OrdinalIgnoreCase) -ge 0 } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`

// reapStale kills the orphan via a CIM query. It runs under Windows PowerShell
// (`powershell`), which ships with every Windows install — Commander needs
// pwsh 7 for its sessions, but a reap on the startup path should not be the
// thing that depends on it.
//
// The timeout matters: this sits on both the startup and shutdown paths, and a
// wedged WMI service would otherwise hang the app rather than merely fail to
// tidy up.
func reapStale(configPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := proc.Hide(exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-Command", reapScript))
	cmd.Env = append(os.Environ(), reapConfigEnv+"="+configPath)
	_ = cmd.Run()
}
