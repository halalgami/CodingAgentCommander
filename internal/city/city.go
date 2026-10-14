// Package city turns the ledger's aggregates into the Al Modon city model
// (dashboard phase 5a). Pure: the app supplies data and a model→provider
// lookup; the frontend draws the result.
package city

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/progress"
)

const (
	buildingMinMs = 30 * 60 * 1000
	maxBuildings  = 60
	maxTitle      = 120
	oldTownAfter  = 90 * 24 * time.Hour
)

type CityView struct {
	Districts []District `json:"districts"`
	Locked    bool       `json:"locked"`
}

type District struct {
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Rank         string     `json:"rank"`
	Hours        float64    `json:"hours"`
	Plot         int        `json:"plot"`
	Land         int        `json:"land"`
	Roads        int        `json:"roads"`
	Parks        int        `json:"parks"`
	Cracks       int        `json:"cracks"`
	OldTown      bool       `json:"oldTown"`
	Ruins        bool       `json:"ruins"`
	Plaques      []Plaque   `json:"plaques"`
	Buildings    []Building `json:"buildings"`
	Blocks       []Block    `json:"blocks"`
	OldTownCount int        `json:"oldTownCount"`
	Landmarks    []Landmark `json:"landmarks"`
}

type Building struct {
	SessionID string  `json:"sessionID"`
	Title     string  `json:"title"`
	Archetype string  `json:"archetype"`
	Hours     float64 `json:"hours"`
	Seed      uint32  `json:"seed"`
	StartedAt int64   `json:"startedAt"`
}

type Block struct {
	Week     string  `json:"week"`
	Hours    float64 `json:"hours"`
	Sessions int     `json:"sessions"`
}

type Plaque struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	At   int64  `json:"at"`
}

type Landmark struct {
	Kind      string `json:"kind"` // longest | commits
	SessionID string `json:"sessionID"`
}

type ProjectData struct {
	Info     store.ProjectInfo
	Sessions []store.SessionSummary
	Unlocks  []store.Unlock
}

// ArchetypeOf picks a building type from the catalog provider of a model,
// falling back to the model id when the catalog does not know it.
func ArchetypeOf(provider, model string) string {
	switch {
	case provider == "anthropic":
		return "tower"
	case strings.HasPrefix(provider, "ollama"):
		return "block"
	case provider != "":
		return "hall"
	case strings.HasPrefix(model, "claude"):
		return "tower"
	}
	return "hall"
}

func seedOf(id string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	if s := h.Sum32(); s != 0 {
		return s
	}
	return 1
}

func dominantModel(m map[string]int64) string {
	best, n := "", int64(-1)
	for k, v := range m {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

func hours(ms int64) float64 { return float64(ms) / 3.6e6 }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func district(pd ProjectData, provider func(string) string, now time.Time) District {
	in := pd.Info
	d := District{
		Key: in.Key, Label: parse.Clip(in.Label, maxTitle), Plot: in.Plot, Ruins: in.Missing,
		Hours: hours(in.Totals.ActiveMs), Land: 4,
		Plaques: []Plaque{}, Buildings: []Building{}, Blocks: []Block{}, Landmarks: []Landmark{},
	}
	d.Rank = progress.ProjectRank(d.Hours)
	d.OldTown = in.FirstSeen > 0 && now.Sub(time.UnixMilli(in.FirstSeen)) > oldTownAfter
	if snap := in.Latest; snap != nil && snap.Code > 0 {
		d.Land = clampInt(int(math.Ceil(math.Sqrt(float64(snap.Code)/400))), 4, 24)
		cpk := float64(snap.Complexity) * 1000 / float64(snap.Code)
		switch {
		case cpk >= 300:
			d.Roads = 2
		case cpk >= 150:
			d.Roads = 1
		}
		d.Parks = int(math.Round(snap.TestRatio * float64(d.Land*d.Land) * 0.25))
		todosPerK := float64(snap.Todos) * 1000 / float64(snap.Code)
		d.Cracks = clampInt(int(todosPerK*2), 0, d.Land)
	}
	for _, u := range pd.Unlocks {
		d.Plaques = append(d.Plaques, Plaque{ID: u.ID, Name: progress.NameOf(u.ID), At: u.At})
	}

	var big []store.SessionSummary
	weeks := map[string]*Block{}
	for _, s := range pd.Sessions {
		if s.ActiveMs >= buildingMinMs {
			big = append(big, s)
			continue
		}
		if s.ActiveMs <= 0 {
			continue
		}
		y, w := time.UnixMilli(s.StartedAt).ISOWeek()
		key := fmt.Sprintf("%d-W%02d", y, w)
		b := weeks[key]
		if b == nil {
			b = &Block{Week: key}
			weeks[key] = b
		}
		b.Hours += hours(s.ActiveMs)
		b.Sessions++
	}
	sort.Slice(big, func(i, j int) bool {
		if big[i].ActiveMs != big[j].ActiveMs {
			return big[i].ActiveMs > big[j].ActiveMs
		}
		return big[i].ID < big[j].ID
	})
	var longest, commits store.SessionSummary
	for i, s := range big {
		if s.ActiveMs > longest.ActiveMs {
			longest = s
		}
		if s.Commits > commits.Commits {
			commits = s
		}
		if i >= maxBuildings {
			d.OldTownCount++
			continue
		}
		m := dominantModel(s.Models)
		d.Buildings = append(d.Buildings, Building{
			SessionID: s.ID, Title: parse.Clip(s.Title, maxTitle), Archetype: ArchetypeOf(provider(m), m),
			Hours: hours(s.ActiveMs), Seed: seedOf(s.ID), StartedAt: s.StartedAt,
		})
	}
	if longest.ID != "" {
		d.Landmarks = append(d.Landmarks, Landmark{Kind: "longest", SessionID: longest.ID})
	}
	if commits.Commits >= 5 {
		d.Landmarks = append(d.Landmarks, Landmark{Kind: "commits", SessionID: commits.ID})
	}
	for _, b := range weeks {
		d.Blocks = append(d.Blocks, *b)
	}
	sort.Slice(d.Blocks, func(i, j int) bool { return d.Blocks[i].Week < d.Blocks[j].Week })
	return d
}

// Build returns the city for the given projects, districts ordered by plot.
func Build(ps []ProjectData, provider func(model string) string, now time.Time) CityView {
	v := CityView{Districts: make([]District, 0, len(ps))}
	for _, pd := range ps {
		v.Districts = append(v.Districts, district(pd, provider, now))
	}
	sort.Slice(v.Districts, func(i, j int) bool { return v.Districts[i].Plot < v.Districts[j].Plot })
	return v
}
