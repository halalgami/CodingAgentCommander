package live

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func ev(k parse.Kind, at int64, f ...func(*parse.Event)) parse.Event {
	e := parse.Event{Kind: k, At: at, SessionID: "s1"}
	for _, fn := range f {
		fn(&e)
	}
	return e
}

func tool(id, name, target string) func(*parse.Event) {
	return func(e *parse.Event) { e.ToolID, e.ToolName, e.Target = id, name, target }
}

func TestTurnLifecycle(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Prompt, 1))
	if s.State != Thinking || !s.TurnOpen || s.SinceMs != 1 {
		t.Fatalf("after prompt: %+v", s)
	}
	s = Reduce(s, ev(parse.Assistant, 2, func(e *parse.Event) { e.HasUsage = true; e.Usage = parse.Usage{In: 1, CacheRead: 2, CacheWrite: 3} }))
	if s.State != Thinking || s.SinceMs != 1 || s.ContextTokens != 6 || s.Messages != 1 {
		t.Fatalf("thinking must keep its start time and track context: %+v", s)
	}
	s = Reduce(s, ev(parse.ToolUse, 3, tool("t1", "Edit", "app.go")))
	if s.State != Tool || s.Tool != "Edit" || s.Target != "app.go" || s.SinceMs != 3 {
		t.Fatalf("tool: %+v", s)
	}
	s = Reduce(s, ev(parse.ToolResult, 4, func(e *parse.Event) { e.ToolID = "t1" }))
	if s.State != Thinking || len(s.Pending) != 0 {
		t.Fatalf("result closes tool: %+v", s)
	}
	s = Reduce(s, ev(parse.TurnDuration, 5))
	if s.State != Finished || !s.Unread || s.TurnOpen {
		t.Fatalf("turn end: %+v", s)
	}
	s = Ack(s, 6)
	if s.State != Idle || s.Unread {
		t.Fatalf("ack: %+v", s)
	}
}

func TestPermissionOnlyWhileToolPending(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.ToolUse, 1, tool("t1", "Bash", "rm -rf build")))
	s = ReduceHook(s, HookPermission, 7)
	if s.State != Permission || s.Tool != "Bash" {
		t.Fatalf("permission: %+v", s)
	}
	s = Reduce(s, ev(parse.ToolResult, 9, func(e *parse.Event) { e.ToolID = "t1" }))
	if s.State != Thinking {
		t.Fatalf("approval clears permission: %+v", s)
	}
	// A late permission hook (the ~6 s delay) after the tool already finished must not resurrect it.
	s = ReduceHook(s, HookPermission, 10)
	if s.State != Thinking {
		t.Fatalf("stale permission hook applied: %+v", s)
	}
}

func TestStopHookAndIdlePrompt(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Prompt, 1))
	s = ReduceHook(s, HookStop, 2)
	if s.State != Finished || !s.Unread {
		t.Fatalf("stop: %+v", s)
	}
	s = ReduceHook(s, HookIdle, 3)
	if s.State != Finished {
		t.Fatalf("idle_prompt must not hide an unread finish: %+v", s)
	}
	s = Ack(s, 4)
	s = ReduceHook(s, HookIdle, 5)
	if s.State != Idle {
		t.Fatalf("idle: %+v", s)
	}
}

func TestAPIErrorAndSidechain(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Prompt, 1))
	s = Reduce(s, ev(parse.ToolUse, 2, tool("t9", "Read", "x"), func(e *parse.Event) { e.Sidechain = true }))
	if s.State != Thinking {
		t.Fatalf("sidechain tool must not change main state: %+v", s)
	}
	s = Reduce(s, ev(parse.APIError, 3))
	if s.State != Error || s.TurnOpen {
		t.Fatalf("api error: %+v", s)
	}
}

func TestTitleAndSummary(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Title, 0, func(e *parse.Event) { e.Text = "Fix VAT rounding" }))
	s = Reduce(s, ev(parse.AwaySummary, 2, func(e *parse.Event) { e.Text = "Rebuilding" }))
	if s.Title != "Fix VAT rounding" || s.Summary != "Rebuilding" {
		t.Fatalf("%+v", s)
	}
}

func TestInterruptEndsTheTurn(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Prompt, 1))
	s = Reduce(s, ev(parse.ToolUse, 2, tool("t1", "Bash", "sleep 100")))
	s = Reduce(s, ev(parse.Interrupted, 3))
	if s.State != Idle || s.TurnOpen || len(s.Pending) != 0 || s.Unread {
		t.Fatalf("after Esc: %+v; want idle, turn closed, nothing pending, not unread", s)
	}
}

func TestTargetPathFollowsTool(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.ToolUse, 1, tool("t1", "Edit", "app.go"), func(e *parse.Event) { e.FilePath = "/repo/app.go" }))
	if s.TargetPath != "/repo/app.go" {
		t.Fatalf("target path %q", s.TargetPath)
	}
	s = Reduce(s, ev(parse.ToolResult, 2, func(e *parse.Event) { e.ToolID = "t1" }))
	if s.TargetPath != "" {
		t.Fatal("target path kept after the tool finished")
	}
}

func TestNotificationResumesLeadWithoutClearingUnread(t *testing.T) {
	s := Session{ID: "s1", State: Finished, Unread: true}
	s = Reduce(s, parse.Event{Kind: parse.Notification, At: 10, SessionID: "s1", AgentID: "a1"})
	if s.State != Thinking || !s.TurnOpen || !s.Unread {
		t.Fatalf("after notification: %+v", s)
	}
}

func TestCompactionStampsTheSession(t *testing.T) {
	var s Session
	s = Reduce(s, ev(parse.Prompt, 1))
	s = Reduce(s, ev(parse.Compact, 50))
	if s.CompactedMs != 50 {
		t.Fatalf("CompactedMs = %d, want 50", s.CompactedMs)
	}
}
