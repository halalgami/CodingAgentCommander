//go:build !windows

package hookmgr

import (
	"os/exec"
	"strings"
	"testing"
)

// The hook runs on PreToolUse, where exit code 2 blocks the tool call, so the
// command must exit 0 whatever curl does.
func TestUnixCommandAlwaysExitsZero(t *testing.T) {
	got := command(1234, "tok")
	if !strings.Contains(got, ">/dev/null 2>&1; exit 0 # "+Sentinel) {
		t.Errorf("missing exit-0 guard: %q", got)
	}
	if !strings.Contains(got, "-m 2") {
		t.Errorf("lost the 2s cap: %q", got)
	}
	// Port 1 refuses the connection: curl exits non-zero, the hook must not.
	if err := exec.Command("sh", "-c", strings.Replace(got, "localhost:1234", "localhost:1", 1)).Run(); err != nil {
		t.Errorf("hook command failed with curl unreachable: %v", err)
	}
}
