//go:build windows

package hookmgr

import (
	"strings"
	"testing"
)

// TestWindowsCommandIsShellPortable guards the constraints that let one hook
// command run under both shells Claude Code may choose on Windows (Git Bash
// preferred, PowerShell as fallback). Each of these silently broke the
// session-finished notification when it was wrong.
func TestWindowsCommandIsShellPortable(t *testing.T) {
	got := command(1234, "tok")

	if strings.Contains(got, "/dev/null") {
		t.Error("uses /dev/null; PowerShell cannot resolve it — use `-o NUL`")
	}
	if !strings.Contains(got, "-o NUL") {
		t.Error("missing `-o NUL`: curl must open the null device itself, not via a shell redirect")
	}
	if strings.Contains(got, "2>&1") {
		t.Error("uses a POSIX stderr redirect, which is not portable to PowerShell")
	}
	if !strings.Contains(got, `-d "@-"`) {
		t.Error(`@- must be quoted: bare @- is a PowerShell parse error (@ begins splatting)`)
	}
	if !strings.HasSuffix(got, "# "+Sentinel) {
		t.Errorf("must end with the `#` sentinel comment (a line comment in both shells); got %q", got)
	}
	if !strings.Contains(got, "token=tok") {
		t.Error("notify command missing auth token")
	}
	if !strings.Contains(got, "localhost:1234") {
		t.Error("notify command missing port")
	}
}
