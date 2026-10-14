package autocont

import (
	"fmt"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Now the implementation:", true},
		{"Let me update the tests.", true},
		{"Found it. Let me fix the handler", true},
		{"Now the implementation:</think>", true},
		{"Now the implementation:\n</think>\n</think>", true},
		{"**Next, the migration:**", true},
		{"Writing the file `main.go`:`", true},
		{"Continue with P1 Task 1?", false},
		{"Done. Let me know if you want more:", false},
		{"I'll report once it's deployed.", false},
		{"", false},
		{"   ", false},
		{"All tests pass and the branch is merged.", false},
		{"let me check:", true},
	}
	for _, c := range cases {
		if got := Detect(c.msg); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestHookReady(t *testing.T) {
	cases := map[string]bool{
		"2.1.296 (Claude Code)": true, "2.1.210 (Claude Code)": true, "2.1.209 (Claude Code)": false,
		"2.2.0": true, "3.0.0": true, "1.9.999": false, "": false, "garbage": false, "2.1": false,
	}
	for v, want := range cases {
		if got := HookReady(v); got != want {
			t.Errorf("HookReady(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestTrackerBlocksThenGivesUpWhenStuck(t *testing.T) {
	tr := NewTracker()
	if v := tr.Decide("s", "p1", true); v != Block {
		t.Fatalf("first pause = %v, want Block", v)
	}
	if tr.Count("s") != 1 {
		t.Fatalf("count = %d", tr.Count("s"))
	}
	if v := tr.Decide("s", "p1", true); v != GaveUpStuck {
		t.Fatalf("pause with no progress = %v, want GaveUpStuck", v)
	}
}

func TestTrackerProgressAllowsAnotherBlock(t *testing.T) {
	tr := NewTracker()
	tr.Decide("s", "p1", true)
	tr.Progress("s")
	if v := tr.Decide("s", "p1", true); v != Block {
		t.Fatalf("pause after progress = %v, want Block", v)
	}
	if tr.Count("s") != 2 {
		t.Fatalf("count = %d", tr.Count("s"))
	}
}

func TestTrackerCap(t *testing.T) {
	tr := NewTracker()
	for i := 0; i < Limit; i++ {
		if v := tr.Decide("s", "p1", true); v != Block {
			t.Fatalf("pause %d = %v, want Block", i+1, v)
		}
		tr.Progress("s")
	}
	if v := tr.Decide("s", "p1", true); v != GaveUpCap {
		t.Fatalf("pause %d = %v, want GaveUpCap", Limit+1, v)
	}
}

func TestTrackerNotPausedAllows(t *testing.T) {
	tr := NewTracker()
	if v := tr.Decide("s", "p1", false); v != Allow {
		t.Fatalf("real stop = %v, want Allow", v)
	}
}

func TestTrackerNewPromptResets(t *testing.T) {
	tr := NewTracker()
	tr.Decide("s", "p1", true)
	if v := tr.Decide("s", "p2", true); v != Block {
		t.Fatalf("new prompt pause = %v, want Block", v)
	}
	if tr.Count("s") != 1 {
		t.Fatalf("count after new prompt = %d", tr.Count("s"))
	}
}

func TestTrackerResetAndDrop(t *testing.T) {
	tr := NewTracker()
	tr.Decide("s", "p1", true)
	tr.Reset("s")
	if tr.Count("s") != 0 {
		t.Fatalf("count after Reset = %d", tr.Count("s"))
	}
	if v := tr.Decide("s", "p1", true); v != Block {
		t.Fatalf("pause after Reset = %v, want Block", v)
	}
	tr.Drop("s")
	if tr.Count("s") != 0 {
		t.Fatalf("count after Drop = %d", tr.Count("s"))
	}
}

func TestTrackerEvictsOldest(t *testing.T) {
	tr := NewTracker()
	for i := 0; i <= maxSessions; i++ {
		tr.Decide(fmt.Sprint("s", i), "p", true)
	}
	if tr.Count("s0") != 0 {
		t.Fatal("oldest session not evicted")
	}
	if tr.Count(fmt.Sprint("s", maxSessions)) != 1 {
		t.Fatal("newest session missing")
	}
}

// Minors sweep: after giving up, a later pause on the same prompt (e.g. Claude
// resuming on its own after a background task) toasted "seems stuck" again.
func TestTrackerGivesUpOncePerPrompt(t *testing.T) {
	tr := NewTracker()
	tr.Decide("s", "p1", true)
	if v := tr.Decide("s", "p1", true); v != GaveUpStuck {
		t.Fatalf("second pause = %v", v)
	}
	if v := tr.Decide("s", "p1", true); v != Allow {
		t.Fatalf("pause after giving up = %v, want a silent Allow", v)
	}
	if v := tr.Decide("s", "p2", true); v != Block {
		t.Fatalf("new prompt = %v, want Block", v)
	}
}

// Minors sweep: a dropped-then-re-added session left a stale id early in the
// eviction order, so eviction removed it instead of the real oldest.
func TestTrackerEvictionIgnoresDroppedIDs(t *testing.T) {
	tr := NewTracker()
	tr.Decide("s0", "p", true)
	tr.Decide("s1", "p", true)
	tr.Drop("s0")
	tr.Decide("s0", "p", true)
	for i := 2; i <= maxSessions; i++ {
		tr.Decide(fmt.Sprint("s", i), "p", true)
	}
	if tr.Count("s1") != 0 || tr.Count("s0") != 1 {
		t.Fatalf("s1 = %d (want evicted), s0 = %d (want kept)", tr.Count("s1"), tr.Count("s0"))
	}
}
