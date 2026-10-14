package progress

import (
	"sort"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
)

type Def struct {
	ID, Name string
	Account  bool
}

var Defs = []Def{
	{"first_blood", "First Blood", false}, {"test_whisperer", "Test Whisperer", false},
	{"night_owl", "Night Owl", false}, {"marathon", "Marathon", false}, {"flawless", "Flawless", false},
	{"big_brain", "Big Brain", false}, {"speedrun", "Speedrun", false}, {"demolition", "Demolition Crew", false},
	{"urban_renewal", "Urban Renewal", false},
	{"commander_10", "Commander 10", true}, {"commander_25", "Commander 25", true}, {"commander_50", "Commander 50", true},
}

func NameOf(id string) string {
	for _, d := range Defs {
		if d.ID == id {
			return d.Name
		}
	}
	return id
}

var sessionRules = []struct {
	id string
	ok func(s store.SessionSummary) bool
}{
	{"first_blood", func(s store.SessionSummary) bool { return s.Commits > 0 }},
	{"test_whisperer", func(s store.SessionSummary) bool { return s.TestRuns-s.TestFails >= 10 }},
	{"night_owl", func(s store.SessionSummary) bool { return s.NightTurns > 0 }},
	{"marathon", func(s store.SessionSummary) bool { return s.ActiveMs >= 3*3600000 }},
	{"flawless", func(s store.SessionSummary) bool { return s.BestCleanStreak >= 20 }},
	{"big_brain", func(s store.SessionSummary) bool { return s.MaxContext > 500000 }},
	{"speedrun", func(s store.SessionSummary) bool { return s.FastCommits > 0 }},
	{"demolition", func(s store.SessionSummary) bool { return s.LinesRemoved-s.LinesAdded >= 200 }},
}

// Evaluate returns project achievements newly satisfied by p (not already unlocked),
// attributed to the earliest qualifying session.
func Evaluate(p store.Project, now int64) []store.Unlock {
	have := map[string]bool{}
	for _, u := range p.Unlocks {
		have[u.ID] = true
	}
	sessions := append([]store.SessionSummary(nil), p.Sessions...)
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].StartedAt < sessions[j].StartedAt })
	var out []store.Unlock
	for _, r := range sessionRules {
		if have[r.id] {
			continue
		}
		for _, s := range sessions {
			if r.ok(s) {
				out = append(out, store.Unlock{ID: r.id, At: now, SessionID: s.ID})
				break
			}
		}
	}
	if !have["urban_renewal"] && urbanRenewal(p) {
		out = append(out, store.Unlock{ID: "urban_renewal", At: now})
	}
	return out
}

// urbanRenewal: latest snapshot's complexity is ≥ 10 % below the newest
// snapshot at least 7 days older.
func urbanRenewal(p store.Project) bool {
	n := len(p.Snapshots)
	if n < 2 {
		return false
	}
	last := p.Snapshots[n-1]
	if last.Partial {
		return false // a capped scan undercounts: not evidence of a drop
	}
	lastDay, err := time.Parse("2006-01-02", last.Day)
	if err != nil {
		return false
	}
	for i := n - 2; i >= 0; i-- {
		d, err := time.Parse("2006-01-02", p.Snapshots[i].Day)
		if err != nil || lastDay.Sub(d) < 7*24*time.Hour || p.Snapshots[i].Partial {
			continue
		}
		prev := p.Snapshots[i].Complexity
		return prev > 0 && float64(last.Complexity) <= 0.9*float64(prev)
	}
	return false
}

// EvaluateAccount returns Commander milestones reached and not yet held.
func EvaluateAccount(level int, have map[string]bool, now int64) []store.Unlock {
	var out []store.Unlock
	for _, m := range []struct {
		id string
		at int
	}{{"commander_10", 10}, {"commander_25", 25}, {"commander_50", 50}} {
		if level >= m.at && !have[m.id] {
			out = append(out, store.Unlock{ID: m.id, At: now})
		}
	}
	return out
}
