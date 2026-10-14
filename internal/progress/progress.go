// Package progress turns ledger counters into XP, levels, ranks and builds.
// Pure: no I/O, no clock.
package progress

import (
	"math"
	"sort"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
)

// Difficulty is 1.0 up to 1 000 total complexity, 1.5 at 10 000, 2.0 at ≥ 100 000.
func Difficulty(totalComplexity int, has bool) float64 {
	if !has || totalComplexity <= 0 {
		return 1
	}
	return 1 + math.Max(0, math.Min(1, (math.Log10(float64(totalComplexity))-3)/2))
}

// SessionXP = (active minutes + 2·turns + 10·commits + 5·green test runs) × difficulty.
func SessionXP(s store.SessionSummary, d float64) float64 {
	green := s.TestRuns - s.TestFails
	if green < 0 {
		green = 0
	}
	base := float64(s.ActiveMs)/60000 + 2*float64(s.Turns) + 10*float64(s.Commits) + 5*float64(green)
	return math.Max(0, base*d)
}

// Level is the largest L with 40·L^1.6 ≤ xp; at least 1, at most 99.
func Level(xp float64) int {
	l := int(math.Pow(xp/40, 1/1.6))
	for l < 99 && 40*math.Pow(float64(l+1), 1.6) <= xp {
		l++
	}
	for l > 1 && 40*math.Pow(float64(l), 1.6) > xp {
		l--
	}
	if l < 1 {
		l = 1
	}
	if l > 99 {
		l = 99
	}
	return l
}

func Title(level int) string {
	switch {
	case level >= 20:
		return "Legend"
	case level >= 15:
		return "Elite"
	case level >= 10:
		return "Veteran"
	case level >= 5:
		return "Adept"
	}
	return "Recruit"
}

func Tier(level int) int {
	switch {
	case level >= 20:
		return 3
	case level >= 10:
		return 2
	case level >= 5:
		return 1
	}
	return 0
}

var families = map[string]string{
	"Edit": "Blade", "Write": "Blade", "MultiEdit": "Blade", "NotebookEdit": "Blade",
	"Read": "Scholar", "Grep": "Scholar", "Glob": "Scholar", "WebFetch": "Scholar", "WebSearch": "Scholar",
	"Bash": "Engineer", "Task": "Summoner", "Agent": "Summoner",
}

// Build names the dominant tool family, or "Top-Second" when the runner-up is ≥ 60 % of it.
func Build(tools map[string]int64) string {
	sum := map[string]int64{}
	for name, n := range tools {
		if f, ok := families[name]; ok {
			sum[f] += n
		}
	}
	type kv struct {
		f string
		n int64
	}
	var r []kv
	for f, n := range sum {
		if n > 0 {
			r = append(r, kv{f, n})
		}
	}
	if len(r) == 0 {
		return ""
	}
	sort.Slice(r, func(i, j int) bool { return r[i].n > r[j].n || r[i].n == r[j].n && r[i].f < r[j].f })
	if len(r) > 1 && float64(r[1].n) >= 0.6*float64(r[0].n) {
		return r[0].f + "-" + r[1].f
	}
	return r[0].f
}

func ProjectRank(hours float64) string {
	switch {
	case hours >= 400:
		return "Capital"
	case hours >= 150:
		return "Citadel"
	case hours >= 50:
		return "Station"
	case hours >= 10:
		return "Settlement"
	}
	return "Outpost"
}

func CommanderLevel(totalXP float64) int {
	return int(math.Floor(math.Sqrt(math.Max(0, totalXP) / 50)))
}

type Loot struct {
	Commits int64 `json:"commits"`
	Lines   int64 `json:"lines"`
	Tests   int64 `json:"tests"`
}

type Hero struct {
	SessionID string           `json:"sessionID"`
	XP        float64          `json:"xp"`
	Level     int              `json:"level"`
	Title     string           `json:"title"`
	Tier      int              `json:"tier"`
	Build     string           `json:"build"`
	Mix       map[string]int64 `json:"mix"`
	Loot      Loot             `json:"loot"`
}

// MixOf sums tool calls per family (Blade, Scholar, Engineer, Summoner); unknown tools are ignored.
func MixOf(tools map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for name, n := range tools {
		if f, ok := families[name]; ok && n > 0 {
			m[f] += n
		}
	}
	return m
}

// LootOf is what a session brings home: commits, lines changed and green test runs.
func LootOf(s store.SessionSummary) Loot {
	return Loot{Commits: s.Commits, Lines: s.LinesAdded + s.LinesRemoved, Tests: max(0, s.TestRuns-s.TestFails)}
}

func HeroOf(s store.SessionSummary, d float64) Hero {
	xp := SessionXP(s, d)
	l := Level(xp)
	return Hero{SessionID: s.ID, XP: xp, Level: l, Title: Title(l), Tier: Tier(l), Build: Build(s.Tools), Mix: MixOf(s.Tools), Loot: LootOf(s)}
}
