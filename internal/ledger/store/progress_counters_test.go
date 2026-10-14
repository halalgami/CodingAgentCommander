package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

func local(h, m, s int) int64 { return time.Date(2026, 10, 11, h, m, s, 0, time.Local).UnixMilli() }

func TestProgressCounters(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	evs := []parse.Event{
		{Kind: parse.Prompt, At: local(2, 0, 0)},
		{Kind: parse.Assistant, At: local(2, 0, 1), MessageID: "m1", HasUsage: true, Usage: parse.Usage{In: 100, CacheRead: 600000}},
		{Kind: parse.ToolUse, At: local(2, 0, 30), ToolID: "c1", ToolName: "Bash", Command: "git commit -m x"},
		{Kind: parse.TurnDuration, At: local(2, 1, 0), UUID: "t1", Ms: 60000},
		{Kind: parse.Prompt, At: local(10, 0, 0)},
		{Kind: parse.ToolUse, At: local(10, 5, 0), ToolID: "c2", ToolName: "Bash", Command: "git commit -m y"},
		{Kind: parse.TurnDuration, At: local(10, 6, 0), UUID: "t2", Ms: 60000},
		{Kind: parse.ToolResult, At: local(10, 7, 0), ToolID: "x", IsError: true},
		{Kind: parse.TurnDuration, At: local(10, 8, 0), UUID: "t3", Ms: 60000},
	}
	for _, e := range evs {
		p.fold(s, e, false, true)
	}
	if s.MaxContext != 600100 {
		t.Errorf("MaxContext %d", s.MaxContext)
	}
	if s.NightTurns != 1 {
		t.Errorf("NightTurns %d (02:01 counts, 10:06 doesn't)", s.NightTurns)
	}
	if s.FastCommits != 1 {
		t.Errorf("FastCommits %d (30 s after prompt counts, 5 min doesn't)", s.FastCommits)
	}
	if s.BestCleanStreak != 2 || s.CleanStreak != 1 {
		t.Errorf("streak cur=%d best=%d; want 1 and 2", s.CleanStreak, s.BestCleanStreak)
	}
}

func TestForkedTurnsDoNotCountForProgress(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	night := parse.Event{Kind: parse.TurnDuration, At: local(1, 0, 0), UUID: "t1", Ms: 1000}
	p.fold(p.session("orig"), night, false, true)
	fork := p.session("fork")
	p.fold(fork, night, false, true)
	if fork.NightTurns != 0 || fork.BestCleanStreak != 0 {
		t.Fatalf("fork counted a copied turn: %+v", fork)
	}
}

func TestSubagentOrEarlierCommitIsNotFast(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	p.fold(s, parse.Event{Kind: parse.Prompt, At: local(12, 0, 0)}, false, true)
	// Read order: the parent's last prompt is known before an older subagent commit.
	p.fold(s, parse.Event{Kind: parse.ToolUse, At: local(11, 0, 0), ToolID: "c", ToolName: "Bash", Command: "git commit -m a"}, true, true)
	p.fold(s, parse.Event{Kind: parse.ToolUse, At: local(11, 59, 0), ToolID: "d", ToolName: "Bash", Command: "git commit -m b"}, false, true)
	if s.FastCommits != 0 {
		t.Fatalf("FastCommits %d; a commit before its prompt, or from a subagent, is not fast", s.FastCommits)
	}
}

func TestMergeKeepsUnlocks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	defer s.Close()
	pathID := repoid.ID{Key: repoid.KeyOf("path:/repo"), Identity: "path:/repo", Root: "/repo", Label: "repo"}
	s.Apply(pathID, "/t/a.jsonl", Cursor{SessionID: "a"}, false, []parse.Event{turn("u1")}, 0)
	s.RecordUnlocks(pathID.Key, []Unlock{{ID: "marathon", At: 5, SessionID: "a"}})
	to := repoID("/repo")
	s.Apply(to, "/t/b.jsonl", Cursor{SessionID: "b"}, false, []parse.Event{turn("u2")}, 0)
	s.RecordUnlocks(to.Key, []Unlock{{ID: "marathon", At: 9}})
	s.Merge(pathID.Key, to)
	us := s.Unlocks(to.Key)
	if len(us) != 1 || us[0].At != 5 {
		t.Fatalf("unlocks after merge %v; want one marathon keeping the earlier At", us)
	}
}

func TestNotificationIsNotAPromptForSpeedrun(t *testing.T) {
	p := &Project{Key: "k", Root: "/repo"}
	s := p.session("s1")
	for _, e := range []parse.Event{
		{Kind: parse.Prompt, At: local(9, 0, 0)},
		{Kind: parse.Notification, At: local(9, 30, 0), AgentID: "a1"},
		{Kind: parse.ToolUse, At: local(9, 30, 20), ToolID: "c1", ToolName: "Bash", Command: "git commit -m x"},
	} {
		p.fold(s, e, false, true)
	}
	if s.FastCommits != 0 {
		t.Fatalf("a task notification restarted the Speedrun clock: FastCommits=%d", s.FastCommits)
	}
}
