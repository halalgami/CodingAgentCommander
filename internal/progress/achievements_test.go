package progress

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

func ids(us []store.Unlock) map[string]string {
	m := map[string]string{}
	for _, u := range us {
		m[u.ID] = u.SessionID
	}
	return m
}

func TestEvaluateEachAchievement(t *testing.T) {
	hot := store.SessionSummary{ID: "a", StartedAt: 2,
		Counters:   store.Counters{Commits: 1, TestRuns: 12, TestFails: 1, ActiveMs: 3 * 3600000, LinesRemoved: 500, LinesAdded: 100},
		NightTurns: 1, BestCleanStreak: 20, MaxContext: 500001, FastCommits: 1}
	cold := store.SessionSummary{ID: "b", StartedAt: 1, Counters: store.Counters{ActiveMs: 1000}}
	p := store.Project{Sessions: []store.SessionSummary{cold, hot},
		Snapshots: []scan.Snapshot{{Day: "2026-10-01", Complexity: 1000}, {Day: "2026-10-11", Complexity: 890}}}
	got := ids(Evaluate(p, 99))
	for _, id := range []string{"first_blood", "test_whisperer", "night_owl", "marathon", "flawless", "big_brain", "speedrun", "demolition", "urban_renewal"} {
		if _, ok := got[id]; !ok {
			t.Errorf("%s not unlocked", id)
		}
	}
	if got["first_blood"] != "a" || got["urban_renewal"] != "" {
		t.Errorf("attribution %v", got)
	}

	none := store.Project{Sessions: []store.SessionSummary{cold},
		Snapshots: []scan.Snapshot{{Day: "2026-10-08", Complexity: 1000}, {Day: "2026-10-11", Complexity: 800}}}
	if g := Evaluate(none, 99); len(g) != 0 {
		t.Errorf("negative case unlocked %v (urban renewal needs a snapshot ≥ 7 days older)", ids(g))
	}
}

func TestEvaluateSkipsExistingAndAccount(t *testing.T) {
	p := store.Project{Sessions: []store.SessionSummary{{ID: "a", Counters: store.Counters{Commits: 1}}},
		Unlocks: []store.Unlock{{ID: "first_blood"}}}
	if g := Evaluate(p, 1); len(g) != 0 {
		t.Fatalf("re-unlocked %v", ids(g))
	}
	acc := ids(EvaluateAccount(26, map[string]bool{"commander_10": true}, 1))
	if _, ok := acc["commander_25"]; !ok || len(acc) != 1 {
		t.Fatalf("account %v", acc)
	}
	if NameOf("first_blood") != "First Blood" {
		t.Fatal("name")
	}
}

func TestPartialSnapshotsNeverUnlockUrbanRenewal(t *testing.T) {
	partialNow := store.Project{Snapshots: []scan.Snapshot{{Day: "2026-10-01", Complexity: 1000}, {Day: "2026-10-11", Complexity: 100, Partial: true}}}
	partialThen := store.Project{Snapshots: []scan.Snapshot{{Day: "2026-10-01", Complexity: 5000, Partial: true}, {Day: "2026-10-11", Complexity: 1000}}}
	for _, p := range []store.Project{partialNow, partialThen} {
		if got := ids(Evaluate(p, 1)); got["urban_renewal"] != "" || len(got) > 0 {
			t.Fatalf("partial snapshot unlocked %v", got)
		}
	}
}
