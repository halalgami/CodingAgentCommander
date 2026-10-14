package city

import (
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

var now = time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local)

func sess(id string, minutes int, model string, startedDaysAgo int) store.SessionSummary {
	s := store.SessionSummary{ID: id, Title: "t " + id, StartedAt: now.AddDate(0, 0, -startedDaysAgo).UnixMilli(), Models: map[string]int64{model: 5}}
	s.ActiveMs = int64(minutes) * 60000
	return s
}

func noCatalog(string) string { return "" }

func TestArchetypes(t *testing.T) {
	cases := []struct{ provider, model, want string }{
		{"anthropic", "claude-opus-5-5", "tower"},
		{"ollama-cloud", "glm-5.3", "block"},
		{"bedrock", "x", "hall"},
		{"", "claude-sonnet-5-5", "tower"},
		{"", "mystery", "hall"},
	}
	for _, c := range cases {
		if got := ArchetypeOf(c.provider, c.model); got != c.want {
			t.Errorf("ArchetypeOf(%q,%q)=%s want %s", c.provider, c.model, got, c.want)
		}
	}
}

func TestBuildingsThresholdWeeklyBlocksAndCap(t *testing.T) {
	var ss []store.SessionSummary
	for i := 0; i < 70; i++ {
		ss = append(ss, sess("big"+string(rune('A'+i%26))+string(rune('a'+i/26)), 30+i, "claude-opus-5-5", i))
	}
	ss = append(ss, sess("tiny1", 10, "claude-opus-5-5", 0), sess("tiny2", 5, "claude-opus-5-5", 0), sess("tiny3", 20, "claude-opus-5-5", 20))
	v := Build([]ProjectData{{Info: store.ProjectInfo{Key: "k", Label: "app", Plot: 1}, Sessions: ss}}, noCatalog, now)
	d := v.Districts[0]
	if len(d.Buildings) != 60 || d.OldTownCount != 10 {
		t.Fatalf("buildings %d oldTown %d", len(d.Buildings), d.OldTownCount)
	}
	if d.Buildings[0].Hours < d.Buildings[59].Hours {
		t.Fatal("buildings not sorted by hours desc")
	}
	if len(d.Blocks) != 2 || d.Blocks[len(d.Blocks)-1].Sessions != 2 {
		t.Fatalf("weekly blocks %+v", d.Blocks)
	}
	if d.Buildings[0].Archetype != "tower" || d.Buildings[0].Seed == 0 {
		t.Fatalf("building %+v", d.Buildings[0])
	}
}

func TestLandRoadsParksCracks(t *testing.T) {
	info := store.ProjectInfo{Key: "k", Plot: 1, Latest: &scan.Snapshot{Code: 160000, Complexity: 40000, TestRatio: 0.2, Todos: 400}}
	d := Build([]ProjectData{{Info: info}}, noCatalog, now).Districts[0]
	if d.Land != 20 || d.Roads != 1 || d.Parks != 20 || d.Cracks != 5 {
		t.Fatalf("land %d roads %d parks %d cracks %d", d.Land, d.Roads, d.Parks, d.Cracks)
	}
	bare := Build([]ProjectData{{Info: store.ProjectInfo{Key: "k", Plot: 1}}}, noCatalog, now).Districts[0]
	if bare.Land != 4 || bare.Roads != 0 || bare.Parks != 0 || bare.Cracks != 0 {
		t.Fatalf("no-metrics district %+v", bare)
	}
	huge := Build([]ProjectData{{Info: store.ProjectInfo{Key: "k", Plot: 1, Latest: &scan.Snapshot{Code: 9_000_000, Complexity: 9_000_000}}}}, noCatalog, now).Districts[0]
	if huge.Land != 24 || huge.Roads != 2 {
		t.Fatalf("huge %+v", huge)
	}
}

func TestOldTownRuinsPlaquesLandmarksRank(t *testing.T) {
	s1 := sess("long", 300, "claude-opus-5-5", 2)
	s2 := sess("commits", 60, "claude-opus-5-5", 3)
	s2.Commits = 7
	info := store.ProjectInfo{Key: "k", Label: "app", Plot: 3, Missing: true, FirstSeen: now.AddDate(0, 0, -120).UnixMilli(), Totals: store.Counters{ActiveMs: 12 * 3600000}}
	d := Build([]ProjectData{{Info: info, Sessions: []store.SessionSummary{s1, s2}, Unlocks: []store.Unlock{{ID: "first_blood", At: 5}}}}, noCatalog, now).Districts[0]
	if !d.OldTown || !d.Ruins || d.Rank != "Settlement" || d.Plot != 3 {
		t.Fatalf("district %+v", d)
	}
	if len(d.Plaques) != 1 || d.Plaques[0].Name != "First Blood" {
		t.Fatalf("plaques %+v", d.Plaques)
	}
	if len(d.Landmarks) != 2 || d.Landmarks[0].SessionID != "long" || d.Landmarks[1].SessionID != "commits" {
		t.Fatalf("landmarks %+v", d.Landmarks)
	}
}

func TestTitlesCappedAndDistrictsOrderedByPlot(t *testing.T) {
	s := sess("x", 40, "claude-opus-5-5", 0)
	s.Title = strings.Repeat("é", 500)
	v := Build([]ProjectData{
		{Info: store.ProjectInfo{Key: "b", Plot: 2}, Sessions: []store.SessionSummary{s}},
		{Info: store.ProjectInfo{Key: "a", Plot: 1}},
	}, noCatalog, now)
	if v.Districts[0].Key != "a" || len([]rune(v.Districts[1].Buildings[0].Title)) > 120 {
		t.Fatalf("order/cap: %+v", v.Districts)
	}
}
