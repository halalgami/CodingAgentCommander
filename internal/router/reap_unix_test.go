//go:build !windows

package router

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReapStale(t *testing.T) {
	if _, err := exec.LookPath("pkill"); err != nil {
		t.Skip("pkill not available")
	}
	// Stand in for an orphaned litellm: a shell whose command line carries a
	// unique marker mimicking the --config path ReapStale matches on. The
	// trailing `:` keeps sh from exec-replacing itself with sleep (which would
	// drop the marker from the live process's argv).
	marker := filepath.Join(t.TempDir(), "litellm.yaml")
	cmd := exec.Command("sh", "-c", "sleep 30; : # "+marker)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start marker proc: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }() // sole Wait() consumer

	ReapStale(marker)

	select {
	case <-done: // process exited => reaped
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("ReapStale did not kill the marked process")
	}

	ReapStale("") // must be a safe no-op
}
