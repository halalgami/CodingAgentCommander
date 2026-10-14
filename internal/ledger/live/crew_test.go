package live

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func agentMeta(tool string, bg bool) AgentMeta {
	shape := "foreground"
	if bg {
		shape = "background"
	}
	return AgentMeta{AgentType: "Explore", Description: "Find the parser", ToolUseID: tool, SpawnDepth: 1, RequestShape: shape, Model: "haiku"}
}

func crewOf(t *testing.T, tr *Tracker, id string) Session {
	t.Helper()
	s, ok := tr.Get(id)
	if !ok {
		t.Fatalf("no session %s", id)
	}
	return s
}

func TestForegroundAgentDelegatesThenFinishesOnResult(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "tu1", ToolName: "Agent", Target: "Find the parser"}}, false)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", false), []parse.Event{
		{Kind: parse.Assistant, At: 2, Model: "claude-haiku"},
		{Kind: parse.ToolUse, At: 3, ToolID: "x1", ToolName: "Grep", Target: "Classify"},
	}, false)
	s := crewOf(t, tr, "s1")
	if len(s.Crew) != 1 || s.Delegating != 1 || s.CrewOut != 0 || s.Crew[0].State != AgentTool || s.Crew[0].Tool != "Grep" || s.Crew[0].Description != "Find the parser" {
		t.Fatalf("working crew: %+v", s)
	}
	tr.Apply([]parse.Event{{Kind: parse.ToolResult, At: 9, SessionID: "s1", ToolID: "tu1", Status: "completed"}}, false)
	s = crewOf(t, tr, "s1")
	if s.Crew[0].State != AgentFinished || s.Crew[0].EndedMs != 9 || s.Delegating != 0 {
		t.Fatalf("after result: %+v", s)
	}
}

func TestFailedForegroundResultMarksFailed(t *testing.T) {
	tr := New(nil)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", false), []parse.Event{{Kind: parse.Assistant, At: 2}}, false)
	tr.Apply([]parse.Event{
		{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "tu1", ToolName: "Agent"},
		{Kind: parse.ToolResult, At: 5, SessionID: "s1", ToolID: "tu1", IsError: true},
	}, false)
	if s := crewOf(t, tr, "s1"); s.Crew[0].State != AgentFailed {
		t.Fatalf("got %+v", s.Crew)
	}
}

// Review Focus 1: the launch result of a background agent is immediate.
func TestBackgroundAgentSurvivesAsyncLaunchResult(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{
		{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "tu1", ToolName: "Agent"},
		{Kind: parse.ToolResult, At: 2, SessionID: "s1", ToolID: "tu1", Status: "async_launched", AgentID: "a1"},
		{Kind: parse.TurnDuration, At: 3, SessionID: "s1"},
	}, false)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", true), []parse.Event{{Kind: parse.Assistant, At: 4}}, false)
	s := crewOf(t, tr, "s1")
	if s.Crew[0].State != AgentWorking || s.CrewOut != 1 || s.Delegating != 0 {
		t.Fatalf("background agent after launch: %+v", s)
	}
	tr.Apply([]parse.Event{{Kind: parse.Notification, At: 20, SessionID: "s1", AgentID: "a1", ToolID: "tu1", Status: "completed"}}, false)
	s = crewOf(t, tr, "s1")
	if s.Crew[0].State != AgentFinished || s.CrewOut != 0 || s.State != Thinking {
		t.Fatalf("after notification: %+v", s)
	}
}

// Review Focus 2: the end arrived before the tracker saw the agent's file.
func TestEndBeforeAgentSeenStartsFinished(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{
		{Kind: parse.ToolUse, At: 1, SessionID: "s1", ToolID: "tu1", ToolName: "Agent"},
		{Kind: parse.ToolResult, At: 9, SessionID: "s1", ToolID: "tu1", Status: "completed"},
	}, false)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", false), []parse.Event{{Kind: parse.Assistant, At: 2}, {Kind: parse.Assistant, At: 8}}, false)
	if s := crewOf(t, tr, "s1"); s.Crew[0].State != AgentFinished || s.Delegating != 0 {
		t.Fatalf("late-seen agent: %+v", s)
	}
}

func TestResumedAgentWorksAgain(t *testing.T) {
	tr := New(nil)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", true), []parse.Event{{Kind: parse.Assistant, At: 2}}, false)
	tr.Apply([]parse.Event{{Kind: parse.Notification, At: 5, SessionID: "s1", AgentID: "a1", Status: "completed"}}, false)
	tr.ApplyAgent("s1", "a1", AgentMeta{}, []parse.Event{{Kind: parse.Assistant, At: 30}}, false)
	if s := crewOf(t, tr, "s1"); s.Crew[0].State != AgentWorking || s.CrewOut != 1 {
		t.Fatalf("resumed: %+v", s.Crew)
	}
}

func TestTickQuietFallbackLostAndLinger(t *testing.T) {
	tr := New(nil)
	tr.ApplyAgent("s1", "quiet", agentMeta("t1", true), []parse.Event{{Kind: parse.Assistant, At: 1000, Status: "end_turn"}}, false)
	tr.ApplyAgent("s1", "busy", agentMeta("t2", true), []parse.Event{{Kind: parse.ToolUse, At: 1000, ToolID: "x", ToolName: "Bash"}}, false)
	tr.Tick(1000 + 30_000)
	s := crewOf(t, tr, "s1")
	if s.Crew[0].State != AgentFinished || s.Crew[1].State != AgentTool {
		t.Fatalf("quiet fallback: %+v", s.Crew)
	}
	tr.Tick(1000 + 10*60_000)
	s = crewOf(t, tr, "s1")
	if len(s.Crew) != 1 || s.Crew[0].ID != "busy" || s.Crew[0].State != AgentLost {
		t.Fatalf("lost + linger: %+v", s.Crew)
	}
	tr.Tick(1000 + 10*60_000 + 60_000)
	if s := crewOf(t, tr, "s1"); len(s.Crew) != 0 {
		t.Fatalf("lost agent not dropped after linger: %+v", s.Crew)
	}
}

func TestAgentTextIsCapped(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	tr := New(nil)
	tr.ApplyAgent("s1", "a1", AgentMeta{AgentType: string(long), Description: string(long)}, nil, false)
	a := crewOf(t, tr, "s1").Crew[0]
	if len([]rune(a.Description)) > 80 || len([]rune(a.Type)) > 40 {
		t.Fatalf("uncapped: %d %d", len(a.Description), len(a.Type))
	}
}

func TestLeadModelFromAssistant(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.Assistant, At: 1, SessionID: "s1", Model: "claude-opus-5-5"}}, true)
	if s := crewOf(t, tr, "s1"); s.Model != "claude-opus-5-5" {
		t.Fatalf("model %q", s.Model)
	}
}

func TestQuietFallbackNeedsEndTurnAndBackground(t *testing.T) {
	tr := New(nil)
	// A text block streamed before a long tool call: not the end of the turn.
	tr.ApplyAgent("s1", "streaming", agentMeta("t1", true), []parse.Event{{Kind: parse.Assistant, At: 1000}}, false)
	// A foreground agent has a definitive end (the lead's result); no guessing.
	tr.ApplyAgent("s1", "fg", agentMeta("t2", false), []parse.Event{{Kind: parse.Assistant, At: 1000, Status: "end_turn"}}, false)
	tr.Tick(1000 + 90_000)
	for _, a := range crewOf(t, tr, "s1").Crew {
		if a.State != AgentWorking {
			t.Fatalf("agent %s ended by the quiet fallback: %+v", a.ID, a)
		}
	}
}

func TestAgentOnlySessionIsOrphanUntilLeadSpeaks(t *testing.T) {
	tr := New(nil)
	tr.ApplyAgent("s1", "a1", AgentMeta{}, []parse.Event{{Kind: parse.Assistant, At: 5000}}, true)
	s := crewOf(t, tr, "s1")
	if !s.Orphan || s.LastEventMs != 5000 {
		t.Fatalf("agent-only session: orphan=%v lastEvent=%d", s.Orphan, s.LastEventMs)
	}
	tr.Apply([]parse.Event{{Kind: parse.Prompt, At: 6000, SessionID: "s1"}}, true)
	if crewOf(t, tr, "s1").Orphan {
		t.Fatal("lead events did not adopt the session")
	}
}

func TestExpireKeepsSessionWithActiveCrewAndReportsRemovals(t *testing.T) {
	tr := New(nil)
	var removed []string
	tr.OnRemove = func(ids []string) { removed = append(removed, ids...) }
	tr.Apply([]parse.Event{{Kind: parse.ToolUse, At: 1000, SessionID: "busy", ToolID: "tu1", ToolName: "Agent"}}, true)
	tr.ApplyAgent("busy", "a1", agentMeta("tu1", false), []parse.Event{{Kind: parse.ToolUse, At: 1000, ToolID: "x", ToolName: "Bash"}}, true)
	tr.Apply([]parse.Event{{Kind: parse.Prompt, At: 1000, SessionID: "idle"}}, true)
	tr.Expire(1000+16*60_000, 15*60_000*1e6)
	if _, ok := tr.Get("busy"); !ok {
		t.Fatal("expired a session whose lead waits on a working agent")
	}
	if _, ok := tr.Get("idle"); ok || len(removed) != 1 || removed[0] != "idle" {
		t.Fatalf("idle not expired or not reported: removed=%v", removed)
	}
	tr.Forget("busy")
	if len(removed) != 2 || removed[1] != "busy" {
		t.Fatalf("Forget not reported: %v", removed)
	}
}

func TestAgentKeepsItsTargetPathInMemory(t *testing.T) {
	tr := New(nil)
	tr.Apply([]parse.Event{{Kind: parse.Prompt, At: 1, SessionID: "s1"}}, false)
	tr.ApplyAgent("s1", "a1", agentMeta("tu1", false), []parse.Event{
		{Kind: parse.ToolUse, At: 2, ToolID: "t1", ToolName: "Edit", Target: "app.go", FilePath: "/r/app.go"},
	}, false)
	s, _ := tr.Get("s1")
	if len(s.Crew) != 1 || s.Crew[0].TargetPath != "/r/app.go" {
		t.Fatalf("crew %+v", s.Crew)
	}
	b, _ := json.Marshal(s.Crew[0])
	if strings.Contains(string(b), "/r/app.go") {
		t.Fatalf("absolute path leaked into JSON: %s", b)
	}
}

// Minors sweep: an agent stamped in the future (clock skew) never timed out,
// so it showed as busy forever. Its clock now starts when we first see it.
func TestFutureStampedAgentStillTimesOut(t *testing.T) {
	tr := New(nil)
	now := int64(1_000_000)
	tr.ApplyAgent("s1", "skewed", agentMeta("t1", true), []parse.Event{{Kind: parse.ToolUse, At: now + 86_400_000, ToolID: "x", ToolName: "Bash"}}, false)
	tr.Tick(now)
	tr.Tick(now + 10*60_000)
	if s := crewOf(t, tr, "s1"); len(s.Crew) != 1 || s.Crew[0].State != AgentLost {
		t.Fatalf("future-stamped agent not lost after 10 min: %+v", s.Crew)
	}
}

// Minors round 2: a finished agent stamped in the future lingered on the strip
// until the clock caught up.
func TestFutureEndedAgentStillLeaves(t *testing.T) {
	tr := New(nil)
	now := int64(1_000_000)
	tr.ApplyAgent("s1", "q", agentMeta("t1", true), []parse.Event{{Kind: parse.Assistant, At: now + 86_400_000, Status: "end_turn"}}, false)
	tr.Tick(now + 86_400_000 + 30_000) // quiet fallback ends it at a future time from this clock's view
	tr.Tick(now)
	tr.Tick(now + 60_000)
	if s := crewOf(t, tr, "s1"); len(s.Crew) != 0 {
		t.Fatalf("future-ended agent still on the strip: %+v", s.Crew)
	}
}
