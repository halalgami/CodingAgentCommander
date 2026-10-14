package live

import (
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func TestTimelineKindsAndThinkReplacedByToolOfSameLine(t *testing.T) {
	tr := New(nil)
	var got []TimelineEvent
	tr.OnEvent = func(evs []TimelineEvent) { got = append(got, evs...) }
	tr.Apply([]parse.Event{
		{Kind: parse.Assistant, At: 1, SessionID: "s1"},
		{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "a", ToolName: "Read", Target: "app.go"},
		{Kind: parse.Assistant, At: 2, SessionID: "s1"},
		{Kind: parse.ToolUse, At: 3, SessionID: "s1", ToolID: "b", ToolName: "Edit", Target: "app.go"},
		{Kind: parse.ToolUse, At: 4, SessionID: "s1", ToolID: "c", ToolName: "Bash", Target: "go test"},
		{Kind: parse.ToolUse, At: 5, SessionID: "s1", ToolID: "d", ToolName: "WebSearch", Target: "q"},
		{Kind: parse.ToolUse, At: 6, SessionID: "s1", ToolID: "e", ToolName: "mcp__x__y"},
	}, false)
	kinds := []string{}
	for _, e := range tr.Timeline()["s1"] {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "read,think,edit,shell,web,other" {
		t.Fatalf("kinds %v", kinds)
	}
	if len(got) == 0 {
		t.Fatal("OnEvent not called")
	}
}

func TestTimelineWaitStartAndEnd(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "a", ToolName: "Bash"}}, false)
	tr.Hook("s1", HookPermission, 2)
	tr.Apply([]parse.Event{{Kind: parse.ToolResult, At: 9, SessionID: "s1", ToolID: "a"}}, false)
	var waits []string
	for _, e := range tr.Timeline()["s1"] {
		if e.Kind == "wait" {
			waits = append(waits, e.Label)
		}
	}
	if strings.Join(waits, ",") != "start,end" {
		t.Fatalf("waits %v", waits)
	}
}

// Review Focus 3: bursts stay within caps and the 10-minute window.
func TestTimelineCapsAndWindow(t *testing.T) {
	tr := New(nil)
	var evs []parse.Event
	for i := 0; i < 1000; i++ {
		evs = append(evs, parse.Event{Kind: parse.ToolUse, At: int64(i) * 1000, SessionID: "s1", ToolID: "x", ToolName: "Read", Target: strings.Repeat("y", 200)})
	}
	tr.Apply(evs, false)
	for i := 0; i < 30; i++ {
		var aev []parse.Event
		for j := 0; j < 200; j++ {
			aev = append(aev, parse.Event{Kind: parse.ToolUse, At: 999_000, ToolID: "z", ToolName: "Grep"})
		}
		tr.ApplyAgent("s1", "a"+string(rune('A'+i)), AgentMeta{}, aev, false)
	}
	tl := tr.Timeline()["s1"]
	lead, perAgent := 0, map[string]int{}
	for _, e := range tl {
		if e.Agent == "" {
			lead++
			if e.At < 999_000-10*60_000 {
				t.Fatalf("event outside window: %d", e.At)
			}
		} else {
			perAgent[e.Agent]++
		}
		if len([]rune(e.Label)) > 60 {
			t.Fatalf("label uncapped")
		}
	}
	if lead > 300 {
		t.Fatalf("lead events %d > 300", lead)
	}
	for a, n := range perAgent {
		if n > 100 {
			t.Fatalf("agent %s events %d > 100", a, n)
		}
	}
}

func TestForgetDropsTimeline(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "a", ToolName: "Read"}}, false)
	tr.Forget("s1")
	if len(tr.Timeline()["s1"]) != 0 {
		t.Fatal("timeline kept after Forget")
	}
}

func TestStaleAgentLanesArePruned(t *testing.T) {
	tr := New(nil)
	tr.ApplyAgent("s1", "old", AgentMeta{}, []parse.Event{{Kind: parse.ToolUse, At: 1000, ToolID: "x", ToolName: "Read"}}, false)
	tr.Apply([]parse.Event{{Kind: parse.ToolUse, At: 1000 + 11*60_000, SessionID: "s1", ToolID: "y", ToolName: "Edit"}}, false)
	for _, e := range tr.Timeline()["s1"] {
		if e.Agent == "old" {
			t.Fatalf("finished agent's lane outlived the window: %+v", e)
		}
	}
	tr.mu.Lock()
	n := len(tr.lines["s1"].agents)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("stale agent lanes kept in memory: %d", n)
	}
}

// Owner, 2026-10-14: a long path in a tool label was clipped from the end, so
// the file name never reached the comms log. Absolute paths keep their tail.
func TestLabelKeepsTheEndOfAbsolutePaths(t *testing.T) {
	e := parse.Event{Kind: parse.ToolUse, ToolName: "Bash", Target: "cat /Users/halalgami/Documents/AlgamProjects/AlAjeerOS/internal/feedback/feedback_never_pipe_a_gate.md | head"}
	if got, want := labelOf(e), "Bash cat …/feedback/feedback_never_pipe_a_gate.md | head"; got != want {
		t.Fatalf("labelOf = %q, want %q", got, want)
	}
	short := parse.Event{Kind: parse.ToolUse, ToolName: "Bash", Target: "ls /tmp/x"}
	if got := labelOf(short); got != "Bash ls /tmp/x" {
		t.Fatalf("short path changed: %q", got)
	}
}
