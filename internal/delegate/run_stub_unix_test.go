//go:build !windows

package delegate

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunSuccessOverwritesMetrics(t *testing.T) {
	useStub(t, "ok")
	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusOK || r.Answer != "found it" {
		t.Fatalf("got %+v", r)
	}
	if r.TokensIn != 15424 || r.TokensOut != 650 {
		t.Errorf("metrics must come from the envelope, got in=%d out=%d", r.TokensIn, r.TokensOut)
	}
	if r.Model != "ollama-glm-5.3-oai" || r.ElapsedMS == 0 || r.Transcript == "" {
		t.Errorf("model/elapsed/transcript must be populated, got %+v", r)
	}
	if !r.Citations[0].Verified {
		t.Error("a citation matching the fixture must be verified by Run")
	}
}

func TestRunAPIErrorIsInfra(t *testing.T) {
	useStub(t, "apierror")
	r, _ := Run(context.Background(), runSpec(t))
	if r.Status != StatusFailedInfra {
		t.Errorf("status = %q, want failed_infra: an api_error_status is transport, not work", r.Status)
	}
	// Without this, a future hang would also report failed_infra and pass
	// this assertion for the wrong reason — only after burning the full 30s
	// deadline set by runSpec.
	if strings.Contains(r.Answer, "deadline") {
		t.Errorf("answer = %q, want the api_error path, not the deadline path", r.Answer)
	}
}

func TestRunUnschemaedResultIsWork(t *testing.T) {
	useStub(t, "badjson")
	r, _ := Run(context.Background(), runSpec(t))
	if r.Status != StatusFailedWork {
		t.Errorf("status = %q, want failed_work: the worker ran and produced garbage", r.Status)
	}
}

func TestRunRecordsPermissionDenials(t *testing.T) {
	useStub(t, "denials")
	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	// The only machine-readable signal that a read-only worker tried to write.
	if r.PermissionDenials != 2 {
		t.Errorf("permission_denials = %d, want 2", r.PermissionDenials)
	}
}

func TestRunAPIErrorPreservesPermissionDenials(t *testing.T) {
	useStub(t, "apierrordenials")
	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	// A run can both attempt a write AND hit a transport error; losing the
	// denial evidence on the infra path loses it exactly when it matters most.
	if r.Status != StatusFailedInfra {
		t.Errorf("status = %q, want failed_infra", r.Status)
	}
	if r.PermissionDenials != 2 {
		t.Errorf("permission_denials = %d, want 2: must survive the infra-failure path", r.PermissionDenials)
	}
	// Without this, a future hang would also land on failed_infra with
	// permission_denials intact and pass this assertion for the wrong
	// reason — only after burning the full 30s deadline set by runSpec.
	if strings.Contains(r.Answer, "deadline") {
		t.Errorf("answer = %q, want the api_error path, not the deadline path", r.Answer)
	}
}

func TestRunGarbledOutputWithProcessErrorIsInfra(t *testing.T) {
	useStub(t, "garbled")
	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	// The envelope itself isn't JSON and the process errored: missing binary,
	// dead proxy, wrong PATH. This is the most common real-world infra failure.
	if r.Status != StatusFailedInfra {
		t.Errorf("status = %q, want failed_infra", r.Status)
	}
	// Without this, a future hang on this stub mode would also report
	// failed_infra and pass this assertion for the wrong reason — only after
	// burning the full 30s deadline set by runSpec.
	if strings.Contains(r.Answer, "deadline") {
		t.Errorf("answer = %q, want the garbled-output path, not the deadline path", r.Answer)
	}
}

func TestRunGarbledOutputCleanExitIsWork(t *testing.T) {
	useStub(t, "garbled0")
	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	// The envelope isn't JSON but the process exited 0: the worker ran and
	// wrote nonsense, which is work failure, not infra failure.
	if r.Status != StatusFailedWork {
		t.Errorf("status = %q, want failed_work", r.Status)
	}
}

func TestRunWritesTranscriptsInsideTheStateOverride(t *testing.T) {
	// The regression that matters: assert WHERE the file lands, not merely
	// that StateDir returns a string. Before StateEnv existed, every stub-driven
	// Run in this suite wrote a transcript into the real
	// ~/.local/state/commander/delegate — 280 files in one day.
	useStub(t, "ok")
	dir := t.TempDir()
	t.Setenv(StateEnv, dir)

	r, err := Run(context.Background(), runSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Transcript == "" {
		t.Fatal("no transcript path returned")
	}
	if !strings.HasPrefix(r.Transcript, dir) {
		t.Errorf("transcript went to %q, outside the override %q — the suite is writing into a real user directory", r.Transcript, dir)
	}
	if _, err := os.Stat(r.Transcript); err != nil {
		t.Errorf("transcript not written: %v", err)
	}
}
