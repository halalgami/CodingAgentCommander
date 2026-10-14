package progress

import (
	"math"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
)

func TestDifficulty(t *testing.T) {
	for _, c := range []struct {
		cx   int
		has  bool
		want float64
	}{{50, true, 1}, {1000, true, 1}, {10000, true, 1.5}, {100000, true, 2}, {500000, true, 2}, {0, false, 1}} {
		if got := Difficulty(c.cx, c.has); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Difficulty(%d,%v)=%v want %v", c.cx, c.has, got, c.want)
		}
	}
	if d := Difficulty(3162, true); d < 1.24 || d > 1.26 {
		t.Errorf("Difficulty(3162)=%v want ~1.25", d)
	}
}

func TestXPLevelTitleTier(t *testing.T) {
	s := store.SessionSummary{Counters: store.Counters{ActiveMs: 60 * 60000, Turns: 10, Commits: 2, TestRuns: 5, TestFails: 1}}
	if xp := SessionXP(s, 1.5); xp != (60+20+20+20)*1.5 {
		t.Fatalf("xp %v", xp)
	}
	for _, c := range []struct {
		xp    float64
		level int
	}{{0, 1}, {121, 1}, {122, 2}, {525, 4}, {526, 5}, {1593, 10}, {4828, 20}, {20913, 50}, {1e12, 99}} {
		if got := Level(c.xp); got != c.level {
			t.Errorf("Level(%v)=%d want %d", c.xp, got, c.level)
		}
	}
	if Title(4) != "Recruit" || Title(5) != "Adept" || Title(12) != "Veteran" || Title(19) != "Elite" || Title(20) != "Legend" {
		t.Error("titles")
	}
	if Tier(4) != 0 || Tier(5) != 1 || Tier(10) != 2 || Tier(20) != 3 {
		t.Error("tiers")
	}
}

func TestBuildRankCommander(t *testing.T) {
	if b := Build(map[string]int64{"Edit": 10, "Write": 5, "Read": 3}); b != "Blade" {
		t.Errorf("build %q", b)
	}
	if b := Build(map[string]int64{"Edit": 10, "Bash": 7}); b != "Blade-Engineer" {
		t.Errorf("build %q", b)
	}
	if b := Build(nil); b != "" {
		t.Errorf("empty build %q", b)
	}
	for h, want := range map[float64]string{5: "Outpost", 10: "Settlement", 100: "Station", 150: "Citadel", 500: "Capital"} {
		if got := ProjectRank(h); got != want {
			t.Errorf("rank(%v)=%s want %s", h, got, want)
		}
	}
	if CommanderLevel(15000) != 17 || CommanderLevel(0) != 0 {
		t.Error("commander")
	}
}

func TestXPNeverNegativeAndMonotonic(t *testing.T) {
	base := store.SessionSummary{Counters: store.Counters{ActiveMs: 1000, Turns: 1, TestRuns: 1, TestFails: 3}}
	if SessionXP(base, 1) < 0 {
		t.Fatal("negative XP")
	}
	more := base
	more.ActiveMs += 60000
	if SessionXP(more, 1) < SessionXP(base, 1) {
		t.Fatal("XP fell as counters grew")
	}
}

func TestHeroMixAndLoot(t *testing.T) {
	var s store.SessionSummary
	s.ID = "s1"
	s.Tools = map[string]int64{"Edit": 4, "Write": 1, "Read": 2, "Grep": 1, "Bash": 3, "Agent": 1, "Weird": 9}
	s.Commits, s.LinesAdded, s.LinesRemoved, s.TestRuns, s.TestFails = 2, 120, 30, 5, 2
	h := HeroOf(s, 1)
	want := map[string]int64{"Blade": 5, "Scholar": 3, "Engineer": 3, "Summoner": 1}
	if len(h.Mix) != len(want) {
		t.Fatalf("mix %v", h.Mix)
	}
	for k, v := range want {
		if h.Mix[k] != v {
			t.Errorf("mix[%s]=%d want %d", k, h.Mix[k], v)
		}
	}
	if h.Loot != (Loot{Commits: 2, Lines: 150, Tests: 3}) {
		t.Errorf("loot %+v", h.Loot)
	}
	s.TestFails = 9
	if HeroOf(s, 1).Loot.Tests != 0 {
		t.Error("green tests must floor at 0")
	}
	if m := MixOf(nil); len(m) != 0 {
		t.Errorf("empty mix %v", m)
	}
}
