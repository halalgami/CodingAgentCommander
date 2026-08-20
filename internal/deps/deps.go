// Package deps resolves the external tools Commander shells out to — tmux (the
// psmux shim on Windows), pwsh 7, and the claude CLI — and, on Windows,
// provides the ones the release can supply itself.
//
// It exists because of a failure mode that is invisible from the code: a
// dependency can be "installed" by a package manager while its entry point is
// gone. A winget package whose Links shim is missing, or a scoop install whose
// shim dir never made it onto PATH, leaves `exec.LookPath("tmux")` failing and
// the user staring at `exec: "tmux": executable file not found in %PATH%` with
// a package manager insisting the thing is installed. Bundling psmux into the
// exe removes that whole class of failure for the tool Commander cannot run
// without; Status lets the UI name what is missing instead of surfacing a raw
// exec error.
package deps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// ManagedDir is the app-owned root for tools Commander provides itself. It sits
// alongside router.ManagedRuntimeDir under the user config dir (Windows:
// %AppData%; macOS: ~/Library/Application Support) so it is per-user and
// survives replacing the portable exe — which is the whole install story on
// Windows.
func ManagedDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "Commander")
}

// Tool is one external dependency as the UI sees it.
type Tool struct {
	Name string `json:"name"` // binary name, e.g. "tmux"
	// Label is what the UI shows: the binary alone is not self-explanatory
	// ("tmux" on Windows is really psmux).
	Label      string `json:"label"`
	Found      bool   `json:"found"`
	Path       string `json:"path"`       // resolved location when Found
	Version    string `json:"version"`    // best-effort, empty when not probed
	Managed    bool   `json:"managed"`    // resolved to a copy Commander provides
	CanInstall bool   `json:"canInstall"` // Commander can fetch it on demand
	Required   bool   `json:"required"`   // sessions cannot launch without it
	Hint       string `json:"hint"`       // install command, shown when missing
}

// lookPath resolves name on the current PATH. ensureLoginPATH has already
// widened PATH by the time anything calls this, so a miss here is a genuine
// absence rather than a Finder/Explorer-launch artefact.
func lookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}

// isExecFile reports whether p is an existing regular file. Windows carries no
// executable bit, so existence is all that can be checked portably; callers use
// it to test for a specific binary they laid down themselves.
func isExecFile(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// prependPATH puts dir at the front of this process's PATH if it is not already
// there. Front rather than back because a copy Commander installed on purpose
// should win over whatever a stale package-manager shim points at. Children
// inherit it, which is the point: the psmux server Commander spawns is what
// actually needs to find pwsh.
func prependPATH(dir string) {
	if dir == "" {
		return
	}
	cur := os.Getenv("PATH")
	for _, p := range filepath.SplitList(cur) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return
		}
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+cur)
}

// writeAtomic writes data to path via a sibling temp file and a rename, so a
// half-written binary is never visible under a name that is about to be on PATH
// — an interrupted extraction would otherwise leave a truncated tmux.exe that
// fails in a far more confusing way than a missing one.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// Both cleanups are no-ops on the success path (the file has been closed and
	// renamed away); on every error path they remove the partial file.
	defer func() {
		tmp.Close()
		os.Remove(name)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// underManagedDir reports whether p lives in the app-owned tool tree, i.e. it is
// a copy Commander laid down rather than one the user installed.
func underManagedDir(p string) bool {
	root, err := filepath.Abs(ManagedDir())
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	// Rel returns a ..-prefixed path when abs is outside root. filepath.Rel is
	// already case-insensitive on Windows, where it matters.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// probeVersion runs `bin args…` and returns its first output line, trimmed.
// Best-effort: a tool that cannot be run reports no version rather than failing
// Status, which must stay callable even when the environment is broken.
func probeVersion(bin string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := proc.Hide(exec.CommandContext(ctx, bin, args...)).Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
}

// claudeTool describes the claude CLI. It stays a prerequisite on every
// platform: it is a self-updating npm package that owns its own OAuth login, so
// freezing a copy inside our exe would break its updater and strand users on
// whatever version we shipped.
func claudeTool() Tool {
	t := Tool{
		Name:     "claude",
		Label:    "claude CLI",
		Required: true,
		Hint:     "npm install -g @anthropic-ai/claude-code",
	}
	t.Path, t.Found = lookPath("claude")
	return t
}

// Missing returns the required tools that are absent, in Status order — the
// list the UI turns into a blocking prompt.
func Missing() []Tool {
	var out []Tool
	for _, t := range Status() {
		if t.Required && !t.Found {
			out = append(out, t)
		}
	}
	return out
}
