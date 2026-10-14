package store

import (
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func at(h int) int64 { return time.Date(2026, 10, 9, h, 0, 0, 0, time.Local).UnixMilli() }

func sampleEvents() []parse.Event {
	return []parse.Event{
		{Kind: parse.Title, Text: "Fix VAT rounding"},
		{Kind: parse.Prompt, At: at(9)},
		{Kind: parse.Assistant, At: at(9), Model: "claude-opus-5-5", HasUsage: true, Usage: parse.Usage{In: 10, Out: 5}},
		{Kind: parse.ToolUse, At: at(9), ToolID: "e1", ToolName: "Edit", FilePath: "/repo/app.go", LinesAdded: 3, LinesRemoved: 1},
		{Kind: parse.ToolUse, At: at(9), ToolID: "b1", ToolName: "Bash", Command: "go test ./..."},
		{Kind: parse.ToolResult, At: at(9), ToolID: "b1", IsError: true},
		{Kind: parse.ToolUse, At: at(10), ToolID: "b2", ToolName: "Bash", Command: "git commit -m x"},
		{Kind: parse.ToolUse, At: at(10), ToolID: "a1", ToolName: "Agent"},
		{Kind: parse.TurnDuration, At: at(10), UUID: "td1", Ms: 60000},
		{Kind: parse.Compact, At: at(10)},
	}
}

func TestFoldAndRecompute(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	for _, e := range sampleEvents() {
		p.fold(s, e, false, true)
	}
	p.recompute()
	tot := p.Totals
	if tot.ActiveMs != 60000 || tot.Turns != 1 || tot.Sessions != 1 {
		t.Fatalf("time/turns: %+v", tot)
	}
	if tot.Tools["Edit"] != 1 || tot.Tools["Bash"] != 2 || tot.Subagents != 1 || tot.Commits != 1 {
		t.Fatalf("tools: %+v", tot)
	}
	if tot.TestRuns != 1 || tot.TestFails != 1 || tot.Errors != 1 || tot.Compactions != 1 {
		t.Fatalf("tests/errors: %+v", tot)
	}
	if tot.LinesAdded != 3 || tot.LinesRemoved != 1 || tot.Tokens.In != 10 {
		t.Fatalf("lines/tokens: %+v", tot)
	}
	if p.Files["app.go"].Edits != 1 {
		t.Fatalf("file touch keyed repo-relative: %+v", p.Files)
	}
	if got := p.Sessions[0]; got.Title != "Fix VAT rounding" || got.TokensByModel["claude-opus-5-5"].Out != 5 {
		t.Fatalf("session: %+v", got)
	}
	if len(p.Days) != 1 || p.Days[time.UnixMilli(at(9)).Format("2006-01-02")].ActiveMs != 60000 {
		t.Fatalf("days: %+v", p.Days)
	}
}

func TestTitlesNotStoredWithoutConsent(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	p.fold(s, parse.Event{Kind: parse.Title, Text: "secret plan"}, false, false)
	if s.Title != "" {
		t.Fatal("title stored although storeTitles is off")
	}
}

func TestForkedTurnsCountedOnceAndResetAllowsRederive(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	turn := parse.Event{Kind: parse.TurnDuration, At: at(9), UUID: "td1", Ms: 1000}
	p.fold(p.session("orig"), turn, false, true)
	p.fold(p.session("fork"), turn, false, true) // copied by --fork-session
	p.recompute()
	if p.Totals.ActiveMs != 1000 {
		t.Fatalf("fork double-counted: %d", p.Totals.ActiveMs)
	}
	p.resetSession("orig")
	p.fold(p.session("orig"), turn, false, true)
	p.recompute()
	if p.Totals.ActiveMs != 1000 || len(p.Sessions) != 2 {
		t.Fatalf("re-derive after reset: %d ms, %d sessions", p.Totals.ActiveMs, len(p.Sessions))
	}
}

func TestSubagentFoldsToolsNotTime(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	p.fold(s, parse.Event{Kind: parse.ToolUse, At: at(9), ToolID: "x", ToolName: "Read", FilePath: "/repo/a.go"}, true, true)
	p.fold(s, parse.Event{Kind: parse.TurnDuration, At: at(9), UUID: "sub", Ms: 5000}, true, true)
	p.recompute()
	if p.Totals.Tools["Read"] != 1 || p.Totals.ActiveMs != 0 {
		t.Fatalf("subagent: %+v", p.Totals)
	}
}

func TestSplitAssistantLinesCountTokensOnce(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	u := parse.Usage{In: 100, Out: 7, CacheRead: 1000}
	// Claude Code writes one message as several lines (thinking, text, tool_use),
	// each repeating the message id and its usage.
	for i := 0; i < 3; i++ {
		p.fold(s, parse.Event{Kind: parse.Assistant, At: at(9), Model: "m", MessageID: "msg_1", HasUsage: true, Usage: u}, false, true)
	}
	p.fold(s, parse.Event{Kind: parse.Assistant, At: at(9), Model: "m", MessageID: "msg_2", HasUsage: true, Usage: u}, false, true)
	p.recompute()
	if got := p.Totals.Tokens.In; got != 200 {
		t.Fatalf("tokens.in = %d, want 200 (two messages)", got)
	}
	if got := s.Models["m"]; got != 2 {
		t.Fatalf("models[m] = %d, want 2 messages", got)
	}
}
