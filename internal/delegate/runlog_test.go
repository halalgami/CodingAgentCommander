package delegate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInvokerForDistinguishesAgentFromHuman(t *testing.T) {
	// This is the whole point of the log: a run the orchestrator chose is the
	// only kind that counts toward the adoption question. Claude Code's Bash
	// tool gives the child a PIPE for stdout; a human at a terminal gives it a
	// character device.
	if got := InvokerFor(os.ModeCharDevice); got != InvokerHuman {
		t.Errorf("char device => %q, want %q", got, InvokerHuman)
	}
	if got := InvokerFor(os.ModeNamedPipe); got != InvokerAgent {
		t.Errorf("named pipe => %q, want %q", got, InvokerAgent)
	}
	// A redirect to a regular file is not a terminal either, so it reads as
	// agent-invoked. Note the mirror case, found by driving it: /dev/null IS a
	// character device, so an agent discarding its output would read as human.
	// Both misreads are documented in InvokerFor rather than fixed — neither
	// occurs in the flow being counted, where the Bash tool always gives a pipe.
	if got := InvokerFor(0); got != InvokerAgent {
		t.Errorf("regular file => %q, want %q", got, InvokerAgent)
	}
}

func TestNewRunRecordCountsVerifiedCitations(t *testing.T) {
	res := Result{
		Status: StatusOK, Model: "ollama-glm-5.3-oai",
		ElapsedMS: 48878, TokensIn: 13599, TokensOut: 759,
		PermissionDenials: 1,
		Citations: []Citation{
			{File: "a.go", Line: 1, Quote: "package main", Verified: true},
			{File: "b.go", Line: 2, Quote: "nope", Verified: false},
			{File: "c.go", Line: 3, Quote: "yes", Verified: true},
		},
	}
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	rec := NewRunRecord("scout", res, InvokerAgent, at)

	if rec.Citations != 3 || rec.Verified != 2 {
		t.Errorf("citations=%d verified=%d, want 3 and 2", rec.Citations, rec.Verified)
	}
	if rec.Role != "scout" || rec.Invoker != InvokerAgent || rec.Status != StatusOK {
		t.Errorf("got %+v", rec)
	}
	if rec.Model != "ollama-glm-5.3-oai" || rec.ElapsedMS != 48878 || rec.TokensIn != 13599 || rec.TokensOut != 759 {
		t.Errorf("measured fields must come straight from the result: %+v", rec)
	}
	if rec.PermissionDenials != 1 {
		t.Errorf("denials = %d, want 1: a read-only worker attempting a write is worth seeing in the log", rec.PermissionDenials)
	}
	if rec.At != "2026-09-04T12:00:00Z" {
		t.Errorf("At = %q, want an RFC3339 UTC stamp", rec.At)
	}
}

func TestAppendRunToAppendsOneJSONLinePerRun(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runs.jsonl")
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	for _, inv := range []string{InvokerAgent, InvokerHuman} {
		rec := NewRunRecord("scout", Result{Status: StatusOK}, inv, at)
		if err := AppendRunTo(p, rec); err != nil {
			t.Fatalf("AppendRunTo: %v", err)
		}
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	// Appending, not truncating: the count over a week IS the measurement, so
	// a second run must never erase the first.
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), b)
	}
	var first RunRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 1 is not valid JSON: %v", err)
	}
	if first.Invoker != InvokerAgent {
		t.Errorf("line 1 invoker = %q, want %q — order must be preserved", first.Invoker, InvokerAgent)
	}
}

func TestAppendRunToCreatesParentDir(t *testing.T) {
	// The state dir may not exist on a first-ever run, and logging must never
	// be the reason a run reports failure.
	p := filepath.Join(t.TempDir(), "nested", "deeper", "runs.jsonl")
	if err := AppendRunTo(p, NewRunRecord("scout", Result{Status: StatusOK}, InvokerAgent, time.Now())); err != nil {
		t.Fatalf("AppendRunTo: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("log not created: %v", err)
	}
}
