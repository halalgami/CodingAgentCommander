//go:build windows

package router

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// startMarkerProc launches a long-lived process whose command line carries
// marker, standing in for an orphaned `litellm --config <marker>`. It returns a
// channel that closes when the process exits.
func startMarkerProc(t *testing.T, marker string) (*exec.Cmd, chan error) {
	t.Helper()
	// The marker rides in a comment so PowerShell never tries to interpret the
	// path, but it is still a literal argument, so it appears verbatim in the
	// command line that Win32_Process reports — which is what ReapStale matches.
	cmd := proc.Hide(exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-Command", "Start-Sleep -Seconds 60 # "+marker))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start marker proc: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }() // sole Wait() consumer
	return cmd, done
}

// TestReapStaleKillsOrphan is the regression test for the gap this file closes:
// ReapStale used to shell out to pkill, which does not exist on Windows, so an
// orphaned litellm survived a force-quit and kept holding its port.
func TestReapStaleKillsOrphan(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "litellm.yaml")
	cmd, done := startMarkerProc(t, marker)

	ReapStale(marker)

	select {
	case <-done: // exited => reaped
	case <-time.After(30 * time.Second): // CIM enumeration is not fast
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("ReapStale did not kill the marked process")
	}

	ReapStale("") // must be a safe no-op
}

// TestReapStaleSparesOtherConfigs guards the property that makes this safe to
// run on startup: matching is on the specific generated config path, so a second
// Commander instance serving a different config must survive.
func TestReapStaleSparesOtherConfigs(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "mine", "litellm.yaml")
	theirs := filepath.Join(dir, "theirs", "litellm.yaml")

	victim, victimDone := startMarkerProc(t, mine)
	bystander, bystanderDone := startMarkerProc(t, theirs)
	defer func() {
		_ = bystander.Process.Kill()
		<-bystanderDone
	}()

	ReapStale(mine)

	select {
	case <-victimDone:
	case <-time.After(30 * time.Second):
		_ = victim.Process.Kill()
		<-victimDone
		t.Fatal("ReapStale did not kill the process matching its own config")
	}

	select {
	case <-bystanderDone:
		t.Fatal("ReapStale killed a process serving a different config")
	case <-time.After(2 * time.Second): // still alive, as it must be
	}
}
