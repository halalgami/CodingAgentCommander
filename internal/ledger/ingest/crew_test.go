package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

func agentLines(cwd string) string {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	return strings.Join([]string{
		`{"type":"user","isSidechain":true,"agentId":"a1","timestamp":"` + ts + `","sessionId":"s1","cwd":"` + cwd + `","message":{"role":"user","content":"look"}}`,
		`{"type":"assistant","isSidechain":true,"agentId":"a1","timestamp":"` + ts + `","sessionId":"s1","cwd":"` + cwd + `","message":{"model":"claude-haiku","content":[{"type":"tool_use","id":"g1","name":"Grep","input":{"pattern":"Classify"}}]}}`,
	}, "\n") + "\n"
}

func writeAgent(t *testing.T, root, cwd string, meta string) (lead, agent string) {
	t.Helper()
	lead = filepath.Join(root, "enc", "s1.jsonl")
	os.WriteFile(lead, []byte(strings.Join(fixture(cwd)[:2], "\n")+"\n"), 0o600)
	dir := filepath.Join(root, "enc", "s1", "subagents")
	os.MkdirAll(dir, 0o755)
	agent = filepath.Join(dir, "agent-a1.jsonl")
	os.WriteFile(agent, []byte(agentLines(cwd)), 0o600)
	if meta != "" {
		os.WriteFile(filepath.Join(dir, "agent-a1.meta.json"), []byte(meta), 0o600)
	}
	return
}

func TestSubagentFileBuildsCrew(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	cwd := t.TempDir()
	tr := live.New(nil)
	in := New(root, tr, repoid.NewResolver())
	lead, agent := writeAgent(t, root, cwd, `{"agentType":"Explore","description":"Find the parser","toolUseId":"tu1","spawnDepth":1,"requestShape":"background","model":"haiku"}`)
	in.Watch(lead)
	in.ReadFile(context.Background(), agent, true)
	s, ok := tr.Get("s1")
	if !ok || len(s.Crew) != 1 {
		t.Fatalf("crew = %+v", s)
	}
	a := s.Crew[0]
	if a.ID != "a1" || a.Type != "Explore" || a.Description != "Find the parser" || !a.Background || a.Tool != "Grep" || s.External {
		t.Fatalf("agent = %+v external=%v", a, s.External)
	}
}

func TestMissingOrHugeMetaFallsBack(t *testing.T) {
	for name, meta := range map[string]string{"missing": "", "corrupt": "{not json", "huge": `{"description":"` + strings.Repeat("x", 5000) + `"}`} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "projects")
			tr := live.New(nil)
			in := New(root, tr, repoid.NewResolver())
			_, agent := writeAgent(t, root, t.TempDir(), meta)
			in.ReadFile(context.Background(), agent, true)
			s, _ := tr.Get("s1")
			if len(s.Crew) != 1 || s.Crew[0].Type != "" || s.Crew[0].Description != "" || s.Crew[0].Tool != "Grep" {
				t.Fatalf("fallback crew = %+v", s.Crew)
			}
		})
	}
}

func TestTailReadsRecentSubagentsOfWatchedLead(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	tr := live.New(nil)
	in := New(root, tr, repoid.NewResolver())
	lead, agent := writeAgent(t, root, t.TempDir(), "")
	old := filepath.Join(filepath.Dir(agent), "agent-old.jsonl")
	os.WriteFile(old, []byte(strings.ReplaceAll(agentLines("/x"), "a1", "old")), 0o600)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)
	in.Watch(lead)
	in.Tail(context.Background())
	s, _ := tr.Get("s1")
	if len(s.Crew) != 1 || s.Crew[0].ID != "a1" {
		t.Fatalf("tail crew = %+v (old agent must be skipped)", s.Crew)
	}
}

func TestSubagentTruncationDoesNotForgetLead(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	tr := live.New(nil)
	in := New(root, tr, repoid.NewResolver())
	lead, agent := writeAgent(t, root, t.TempDir(), "")
	in.ReadFile(context.Background(), lead, true)
	in.ReadFile(context.Background(), agent, true)
	os.WriteFile(agent, []byte("\n"), 0o600) // rewritten shorter
	in.ReadFile(context.Background(), agent, true)
	if _, ok := tr.Get("s1"); !ok {
		t.Fatal("a truncated subagent file forgot its lead session")
	}
}
