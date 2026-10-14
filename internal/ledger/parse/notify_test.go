package parse

import (
	"strings"
	"testing"
)

const notifLine = `{"type":"user","origin":{"kind":"task-notification","producer":"session-task"},"timestamp":"2026-10-12T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"<task-notification>\n<task-id>ab6cc5e71c27f99ba</task-id>\n<tool-use-id>toolu_01P5</tool-use-id>\n<status>completed</status>\n<summary>Agent \"x\" finished</summary>\n</task-notification>"}}`

func TestTaskNotificationIsNotAPrompt(t *testing.T) {
	evs := Classify([]byte(notifLine))
	if len(evs) != 1 || evs[0].Kind != Notification || evs[0].AgentID != "ab6cc5e71c27f99ba" ||
		evs[0].ToolID != "toolu_01P5" || evs[0].Status != "completed" {
		t.Fatalf("notification = %+v", evs)
	}
	// Older Claude Code without origin: the content marker alone is enough.
	noOrigin := strings.Replace(notifLine, `"origin":{"kind":"task-notification","producer":"session-task"},`, "", 1)
	if evs := Classify([]byte(noOrigin)); len(evs) != 1 || evs[0].Kind != Notification {
		t.Fatalf("no-origin notification = %+v", evs)
	}
}

// Minors round 2 (phase 4 review): notifications that are not from an agent
// (seen in real transcripts: "<summary>Background security review found…") have
// no task id. They are valid lines, not damage, so history is not "partial".
func TestNonAgentNotificationIsNotPartial(t *testing.T) {
	line := `{"type":"user","origin":{"kind":"task-notification"},"sessionId":"s1","timestamp":"2026-10-12T10:00:00Z","message":{"role":"user","content":"<task-notification>\n<summary>Background security review found: X</summary>\n</task-notification>"}}`
	evs := Classify([]byte(line))
	if len(evs) != 1 || evs[0].Kind != Notification || evs[0].Partial || evs[0].AgentID != "" {
		t.Fatalf("got %+v", evs)
	}
}

func TestAgentLaunchResultCarriesStatus(t *testing.T) {
	async := `{"type":"user","timestamp":"2026-10-12T10:00:00Z","sessionId":"s1","toolUseResult":{"isAsync":true,"status":"async_launched","agentId":"a89a"},"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_9","content":"launched"}]}}`
	evs := Classify([]byte(async))
	if len(evs) != 1 || evs[0].Kind != ToolResult || evs[0].Status != "async_launched" || evs[0].AgentID != "a89a" {
		t.Fatalf("async result = %+v", evs)
	}
	// toolUseResult is a string for many tools: must not break the line.
	str := `{"type":"user","timestamp":"2026-10-12T10:00:00Z","sessionId":"s1","toolUseResult":"Error: boom","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_8","content":"x","is_error":true}]}}`
	if evs := Classify([]byte(str)); len(evs) != 1 || evs[0].Kind != ToolResult || !evs[0].IsError || evs[0].Status != "" {
		t.Fatalf("string toolUseResult = %+v", evs)
	}
}

func TestAssistantCarriesStopReason(t *testing.T) {
	line := `{"type":"assistant","timestamp":"2026-10-12T10:00:00Z","sessionId":"s1","message":{"id":"m1","model":"x","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}`
	evs := Classify([]byte(line))
	if len(evs) != 1 || evs[0].Kind != Assistant || evs[0].Status != "end_turn" {
		t.Fatalf("got %+v", evs)
	}
}
