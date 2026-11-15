package stats

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/rates"
)

type fake struct {
	gen   uint64
	ps    []store.ProjectInfo
	ss    map[string][]store.SessionSummary
	calls int
}

func (f *fake) Gen() uint64                              { return f.gen }
func (f *fake) Projects() []store.ProjectInfo            { return f.ps }
func (f *fake) Sessions(k string) []store.SessionSummary { f.calls++; return f.ss[k] }

const opus, glm = "claude-opus-5-5", "glm-5.3"

func tc(in, out int64) store.TokenCounts { return store.TokenCounts{In: in, Out: out} }

// day builds a session day split between models.
func day(turns, commits int64, by map[string]store.TokenCounts) store.Counters {
	c := store.Counters{Turns: turns, ActiveMs: turns * 1000, Commits: commits, Tools: map[string]int64{"Edit": turns}}
	for m, t := range by {
		c.Tokens.In += t.In
		c.Tokens.Out += t.Out
		if c.TokensByModel == nil {
			c.TokensByModel = map[string]store.TokenCounts{}
		}
		c.TokensByModel[m] = t
	}
	return c
}

// Fixture: 2 projects, days 2026-11-10..12. Day 10 in p2 is pre-v6 (no split).
func fixture() *fake {
	legacy := store.Counters{Turns: 2, ActiveMs: 2000, Tokens: tc(100, 0)}
	hours := day(4, 1, map[string]store.TokenCounts{opus: tc(1000, 0)})
	hours.Hours = make([]store.HourCounters, 24)
	hours.Hours[9] = store.HourCounters{Turns: 3, ActiveMs: 3000, Tokens: 700}
	hours.Hours[14] = store.HourCounters{Turns: 1, ActiveMs: 1000, Tokens: 300}
	return &fake{gen: 1,
		ps: []store.ProjectInfo{{Key: "k1", Label: "Ayina"}, {Key: "k2", Label: "Commander"}},
		ss: map[string][]store.SessionSummary{
			"k1": {{ID: "s1", Days: map[string]store.Counters{
				"2026-11-11": day(4, 2, map[string]store.TokenCounts{opus: tc(3000, 1000), glm: tc(1000, 0)}),
				"2026-11-12": hours, // Thursday
			}}},
			"k2": {{ID: "s2", Days: map[string]store.Counters{
				"2026-11-10": legacy,
				"2026-11-11": day(1, 0, map[string]store.TokenCounts{opus: tc(500, 500)}),
			}}},
		}}
}

func engine(f *fake, s rates.Settings) *Engine {
	return New(f, rates.NewBook(s), func(string) string { return "" },
		func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local) })
}

func TestProviderFallback(t *testing.T) {
	cat := func(m string) string {
		if m == "my-glm" {
			return "zen-custom"
		}
		return ""
	}
	for in, want := range map[string]string{
		"claude-opus-5-5": "anthropic", "us.anthropic.claude-opus-5-5": "bedrock", "anthropic.claude-x": "bedrock",
		"glm-5.3": "ollama-cloud", "Ollama · qwen": "ollama-cloud", "Go · kimi": "opencode-go",
		"zzz": "Other", "my-glm": "zen-custom", "unknown": "Other",
	} {
		if got := ProviderOf(in, cat); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestRunGroupByDaySplitModel(t *testing.T) {
	res, err := engine(fixture(), rates.Settings{}).Run(Query{From: "2026-11-10", To: "2026-11-12", GroupBy: "day", Split: "model", Metric: "tokens"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Key: "2026-11-10", Split: "unknown", Value: 100},
		{Key: "2026-11-11", Split: opus, Value: 5000},
		{Key: "2026-11-11", Split: glm, Value: 1000},
		{Key: "2026-11-12", Split: opus, Value: 1000},
	}
	if len(res.Rows) != len(want) {
		t.Fatalf("rows %+v", res.Rows)
	}
	for i, w := range want {
		if res.Rows[i].Key != w.Key || res.Rows[i].Split != w.Split || res.Rows[i].Value != w.Value {
			t.Fatalf("row %d: %+v want %+v", i, res.Rows[i], w)
		}
	}
}

func TestRunRejectsBadQuery(t *testing.T) {
	e := engine(fixture(), rates.Settings{})
	for _, q := range []Query{
		{GroupBy: "nope", Metric: "tokens"},
		{GroupBy: "day", Metric: "nope"},
		{GroupBy: "model", Split: "model", Metric: "tokens"},
		{GroupBy: "day", Split: "nope", Metric: "tokens"},
		{GroupBy: "tool", Metric: "cost"},
		{GroupBy: "weekday-hour", Metric: "cost"},
		{GroupBy: "day", Metric: "tokens", From: "2026-11-12", To: "2026-11-10"},
		{GroupBy: "day", Metric: "tokens", From: "11/10/2026"},
	} {
		if _, err := e.Run(q); err == nil {
			t.Errorf("accepted %+v", q)
		}
	}
}

func TestCostSkipsUnpricedModels(t *testing.T) {
	res, err := engine(fixture(), rates.Settings{}).Run(Query{From: "2026-11-11", To: "2026-11-11", GroupBy: "project", Metric: "cost"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Rows {
		if r.Key == "k1" {
			// opus 3000 in @4 + 1000 out @20 = 0.012 + 0.02; glm unpriced
			if math.Abs(r.Value-0.032) > 1e-9 || r.Unpriced != 1000 || r.Label != "Ayina" {
				t.Fatalf("k1 %+v", r)
			}
			return
		}
	}
	t.Fatal("k1 missing")
}

func TestLegacyDayWithoutModelSplit(t *testing.T) {
	res, _ := engine(fixture(), rates.Settings{}).Run(Query{From: "2026-11-10", To: "2026-11-10", GroupBy: "model", Metric: "turns"})
	if len(res.Rows) != 1 || res.Rows[0].Key != "unknown" || res.Rows[0].Value != 2 {
		t.Fatalf("%+v", res.Rows)
	}
}

func TestWeekdayHour(t *testing.T) {
	res, err := engine(fixture(), rates.Settings{}).Run(Query{GroupBy: "weekday-hour", Metric: "turns"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, r := range res.Rows {
		got[r.Key] = r.Value
	}
	if got["3-9"] != 3 || got["3-14"] != 1 || len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestOverviewDeltasAndPlanProration(t *testing.T) {
	e := engine(fixture(), rates.Settings{Plans: map[string]rates.Plan{"anthropic": {Name: "Max", MonthlyUSD: 200}}})
	o := e.Overview("2026-11-11", "2026-11-12")
	if o.Cur.Tokens != 7000 || o.Cur.Commits != 3 {
		t.Fatalf("cur %+v", o.Cur)
	}
	wantPlan := 200 * 2 / 30.44
	if math.Abs(o.Cur.PlanUSD-wantPlan) > 1e-9 {
		t.Fatalf("plan %v want %v", o.Cur.PlanUSD, wantPlan)
	}
	if math.Abs(o.Cur.PlanX-o.Cur.CostUSD/wantPlan) > 1e-9 || o.Cur.UnpricedTokens != 1000 {
		t.Fatalf("planx %+v", o.Cur)
	}
	if o.Prev.Tokens != 100 { // 2026-11-09..10
		t.Fatalf("prev %+v", o.Prev)
	}
	if o.Cur.PricedAt == "" {
		t.Fatal("pricedAt missing")
	}
}

func TestOverviewZeroPrevious(t *testing.T) {
	o := engine(&fake{gen: 1}, rates.Settings{}).Overview("2026-11-11", "2026-11-12")
	for _, v := range []float64{o.Prev.CostUSD, o.Prev.PlanX, o.Prev.CacheShare, o.Prev.CacheSavedUSD, o.Cur.CacheShare, o.Cur.PlanX} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v != 0 {
			t.Fatalf("not zero: %+v", o)
		}
	}
}

func TestModelsAttributionByTokenShare(t *testing.T) {
	ms := engine(fixture(), rates.Settings{}).Models("2026-11-11", "2026-11-11", "k1")
	if len(ms) != 2 || ms[0].Model != opus || ms[1].Model != glm {
		t.Fatalf("%+v", ms)
	}
	// k1 day: 4 turns, 2 commits; opus has 4000/5000 of the tokens
	if math.Abs(ms[0].Turns-3.2) > 1e-9 || math.Abs(ms[1].Commits-0.4) > 1e-9 || ms[0].Priced != true || ms[1].Priced {
		t.Fatalf("%+v", ms)
	}
}

func TestCacheInvalidatesOnGen(t *testing.T) {
	f := fixture()
	e := engine(f, rates.Settings{})
	q := Query{GroupBy: "project", Metric: "tokens"}
	e.Run(q)
	n := f.calls
	e.Run(q)
	if f.calls != n {
		t.Fatal("rebuilt without a Gen change")
	}
	f.gen++
	f.ss["k2"] = nil
	res, _ := e.Run(q)
	if f.calls == n || len(res.Rows) != 1 {
		t.Fatalf("stale after Gen bump: %+v", res.Rows)
	}
}

func TestQueryUnder150ms(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	f := &fake{gen: 1, ss: map[string][]store.SessionSummary{}}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	for p := 0; p < 40; p++ {
		k := fmt.Sprintf("k%d", p)
		f.ps = append(f.ps, store.ProjectInfo{Key: k, Label: k})
		for s := 0; s < 30; s++ {
			days := map[string]store.Counters{}
			for d := 0; d < 3; d++ {
				c := day(5, 1, map[string]store.TokenCounts{opus: tc(1000, 100), glm: tc(100, 10), "claude-sonnet-5-5": tc(10, 1)})
				c.Hours = make([]store.HourCounters, 24)
				days[start.AddDate(0, 0, s*3+d).Format("2006-01-02")] = c
			}
			f.ss[k] = append(f.ss[k], store.SessionSummary{ID: fmt.Sprintf("%s-%d", k, s), Days: days})
		}
	}
	e := engine(f, rates.Settings{})
	e.Run(Query{GroupBy: "day", Metric: "tokens"})
	t0 := time.Now()
	for _, q := range []Query{{GroupBy: "day", Split: "model", Metric: "cost"}, {GroupBy: "weekday-hour", Metric: "turns"}, {GroupBy: "project", Metric: "tokens"}} {
		if _, err := e.Run(q); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(t0); d > 150*time.Millisecond {
		t.Fatalf("3 queries took %v", d)
	}
}

func TestDaysExportKeepsWholeCounters(t *testing.T) {
	f := &fake{gen: 1, ps: []store.ProjectInfo{{Key: "k1", Label: "A"}}, ss: map[string][]store.SessionSummary{"k1": {{ID: "s1", Days: map[string]store.Counters{
		"2026-11-11": day(1, 1, map[string]store.TokenCounts{"a": tc(100, 0), "b": tc(100, 0), "c": tc(100, 0)}),
	}}}}}
	var commits, turns int64
	for _, r := range engine(f, rates.Settings{}).Days("", "") {
		commits += r.Commits
		turns += r.Turns
	}
	if commits != 1 || turns != 1 {
		t.Fatalf("export commits %d turns %d, tiles say 1/1", commits, turns)
	}
}

func TestLegacyDayProratedBySessionModels(t *testing.T) {
	legacy := store.Counters{Turns: 2, Tokens: tc(100, 0)}
	f := &fake{gen: 1, ps: []store.ProjectInfo{{Key: "k1"}}, ss: map[string][]store.SessionSummary{"k1": {{ID: "s1",
		TokensByModel: map[string]store.TokenCounts{opus: tc(300, 0), glm: tc(100, 0)},
		Days:          map[string]store.Counters{"2026-11-10": legacy}}}}}
	res, _ := engine(f, rates.Settings{}).Run(Query{GroupBy: "model", Metric: "tokens"})
	got := map[string]float64{}
	for _, r := range res.Rows {
		got[r.Key] = r.Value
	}
	if got[opus] != 75 || got[glm] != 25 || got["unknown"] != 0 {
		t.Fatalf("%v", got)
	}
}
