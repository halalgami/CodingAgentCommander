//go:build !windows

package delegate

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runSpec builds a Spec pointed at a throwaway root. TimeoutSec is generous
// (30s): the stub exits immediately in every mode except "hang", so this
// costs no wall-clock, and a short deadline here was observed flaking under
// load (a real gate run saw TestRunSuccessOverwritesMetrics fail with
// "worker exceeded its 2s deadline" while idle machines never reproduced it).
// Only the timeout test wants a short deadline; it sets one locally, since
// waiting for it IS the point of that test.
func runSpec(t *testing.T) Spec {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Spec{
		Role: Roles()[0], Task: "find it", Root: root,
		ProxyURL: "http://localhost:1", MasterKey: "sk-local",
		WorkerModel: "ollama-glm-5.3-oai",
	}
	s.Role.TimeoutSec = 30
	return s
}

func useStub(t *testing.T, mode string) {
	t.Helper()
	abs, err := filepath.Abs("testdata/stub-claude")
	if err != nil {
		t.Fatal(err)
	}
	old := claudeBin
	claudeBin = abs
	t.Setenv("STUB_MODE", mode)
	t.Cleanup(func() { claudeBin = old })
}

func TestRunTimeoutKillsWholeGroup(t *testing.T) {
	useStub(t, "hang")
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("STUB_CHILD_PIDFILE", pidfile)

	spec := runSpec(t)
	spec.Role.TimeoutSec = 2 // waiting out the deadline is the point of this test

	start := time.Now()
	r, _ := Run(context.Background(), spec)
	if el := time.Since(start); el > 10*time.Second {
		t.Fatalf("deadline not enforced, took %s", el)
	}
	if r.Status != StatusFailedInfra {
		t.Errorf("status = %q, want failed_infra on timeout", r.Status)
	}
	// The stub writes the pidfile before it sleeps, but on a loaded machine
	// that write can lose the race with us reading it. Poll for a bounded
	// period rather than treating one missed read as "never happened" — this
	// assertion is the only proof the whole process group died, so a lost
	// race must fail loud, not skip quiet.
	//
	// An empty or partially-written pidfile parses to pid 0, and
	// syscall.Kill(0, 0) signals the TEST's own process group rather than
	// erroring — which would misreport as "orphan survived: pid 0 still
	// alive" and send a maintainer hunting a process-group bug that does not
	// exist. Keep polling while the parsed pid is <= 0, and fail loud if the
	// content never becomes a valid pid within the deadline.
	deadline := time.Now().Add(2 * time.Second)
	var pid int
	for {
		b, readErr := os.ReadFile(pidfile)
		if readErr == nil {
			if p, atoiErr := strconv.Atoi(strings.TrimSpace(string(b))); atoiErr == nil && p > 0 {
				pid = p
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("stub never recorded a valid child pid in %s: content %q", pidfile, string(b))
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	// proc.Hide sets no process group; without Setpgid + Kill(-pgid) this
	// grandchild outlives the worker.
	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("orphan survived: pid %d still alive after the worker was killed", pid)
	}
}
