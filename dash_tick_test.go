package main

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func TestDashTickBatchesSessionsAndEvents(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	a.ledger.tracker.Apply([]parse.Event{
		{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "x", ToolName: "Read", Target: "a.go"},
		{Kind: parse.ToolUse, At: 2, SessionID: "s1", ToolID: "y", ToolName: "Edit", Target: "a.go"},
		{Kind: parse.ToolUse, At: 2, SessionID: "s2", ToolID: "z", ToolName: "Bash", Target: "go test"},
	}, false)
	a.ledger.flushPending(1)
	if n := countEvents(fe, "dash:tick"); n != 1 {
		t.Fatalf("dash:tick events %d, want exactly 1 per pump step", n)
	}
	a.ledger.flushPending(2)
	if n := countEvents(fe, "dash:tick"); n != 1 {
		t.Fatalf("empty pump step emitted dash:tick (%d)", n)
	}
	if tl := a.DashSnapshot().Timeline["s1"]; len(tl) != 2 {
		t.Fatalf("snapshot timeline %+v", tl)
	}
}

func TestPowerStateUsesProbe(t *testing.T) {
	a := ledgerTestApp(t)
	old := lowPowerFn
	t.Cleanup(func() { lowPowerFn = old })
	lowPowerFn = func() bool { return true }
	if !a.PowerState().LowPower {
		t.Fatal("PowerState ignored the probe")
	}
}

func TestDashTickReportsRemovedSessions(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.Prompt, At: 1, SessionID: "gone"}}, true)
	a.ledger.flushPending(1)
	a.ledger.tracker.Forget("gone")
	a.ledger.flushPending(2)
	if n := countEvents(fe, "dash:tick"); n != 2 {
		t.Fatalf("a removed session did not reach the dashboard (dash:tick=%d)", n)
	}
	a.ledger.pendMu.Lock()
	left := len(a.ledger.pendGone)
	a.ledger.pendMu.Unlock()
	if left != 0 {
		t.Fatal("removed ids not drained")
	}
}
