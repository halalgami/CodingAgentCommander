package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
	"github.com/halalgami/CodingAgentCommander/internal/stats"
)

// fakeRunner answers from a queue and records every prompt.
type fakeRunner struct {
	mu      sync.Mutex
	prompts []string
	answers []string
	err     error
	gate    chan struct{}
}

func (f *fakeRunner) Run(_ context.Context, model, prompt, schema string) (json.RawMessage, error) {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.answers) == 0 {
		return nil, errors.New("no answer queued")
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return json.RawMessage(a), nil
}

func (f *fakeRunner) calls() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.prompts) }

const goodBrief = `{"headline":"Busy week","points":[{"text":"Tokens up","chart":"spend"},{"text":"Most work on p","chart":""}]}`

// aiTestApp: history on, one titled session in a project rooted at a
// recognisable path, last week.
func aiTestApp(t *testing.T) (*App, *fakeRunner) {
	t.Helper()
	a := ledgerTestApp(t)
	st := attachTestStore(t, a)
	day := time.Now().AddDate(0, 0, -7)
	at := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local).UnixMilli()
	root := "/Users/secret/very-private-root"
	id := repoid.ID{Key: repoid.KeyOf("path:" + root), Identity: "path:" + root, Root: root, Label: "p"}
	st.Apply(id, "/t/s1.jsonl", store.Cursor{SessionID: "s1"}, false, []parse.Event{
		{Kind: parse.Title, At: at, SessionID: "s1", Text: "TOP SECRET TITLE"},
		{Kind: parse.ToolUse, At: at + 1, ToolID: "c", ToolName: "Bash", Command: "git commit -m x"},
		{Kind: parse.TurnDuration, At: at + 2, UUID: "t1", Ms: 60000},
	}, 0)
	f := &fakeRunner{}
	a.ai.runner = f
	return a, f
}

func TestBriefWeekBoundaries(t *testing.T) {
	mon := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local) // a Monday
	if f, to := briefWeek(mon); f != "2026-11-09" || to != "2026-11-15" {
		t.Fatalf("Mon 00:00 → %s..%s", f, to)
	}
	sun := time.Date(2026, 11, 15, 23, 59, 0, 0, time.Local)
	if f, to := briefWeek(sun); f != "2026-11-02" || to != "2026-11-08" {
		t.Fatalf("Sun 23:59 → %s..%s", f, to)
	}
}

func TestBriefFactsNoTitlesOrPaths(t *testing.T) {
	a, _ := aiTestApp(t)
	from, to := briefWeek(time.Now())
	b, _ := json.Marshal(briefFacts(a.engine(), from, to))
	s := string(b)
	if strings.Contains(s, "TOP SECRET") || strings.Contains(s, "/Users/secret") {
		t.Fatalf("facts leak: %s", s)
	}
	if !strings.Contains(s, `"p"`) {
		t.Fatalf("facts miss the project label: %s", s)
	}
}

func TestBriefingDoesNotCallModel(t *testing.T) {
	a, f := aiTestApp(t)
	v := a.Briefing()
	if f.calls() != 0 || v.Brief != nil || v.Locked || v.Disclosure == "" {
		t.Fatalf("calls=%d view=%+v", f.calls(), v)
	}
}

func TestRunBriefingCachesPerWeek(t *testing.T) {
	a, f := aiTestApp(t)
	f.answers = []string{goodBrief}
	v := a.RunBriefing()
	if v.Error != "" || v.Brief == nil || v.Brief.Headline != "Busy week" || len(v.Brief.Points) != 2 {
		t.Fatalf("run: %+v", v)
	}
	if !strings.Contains(f.prompts[0], `"p"`) || strings.Contains(f.prompts[0], "TOP SECRET") {
		t.Fatalf("prompt: %s", f.prompts[0])
	}
	if got := a.Briefing(); got.Brief == nil || got.Brief.Headline != "Busy week" || f.calls() != 1 {
		t.Fatalf("cached view %+v calls=%d", got, f.calls())
	}
	// A fresh process reads the cache from disk.
	b := &App{cfg: a.cfg}
	b.ledger = a.ledger
	b.ai.runner = f
	if got := b.Briefing(); got.Brief == nil {
		t.Fatal("cache not persisted")
	}
}

func TestRunBriefingBadOutputNotCached(t *testing.T) {
	a, f := aiTestApp(t)
	f.answers = []string{`{"headline":""}`}
	if v := a.RunBriefing(); v.Error == "" || v.Brief != nil {
		t.Fatalf("bad output accepted: %+v", v)
	}
	f.err = errors.New("model error (status 429): rate")
	if v := a.RunBriefing(); !strings.Contains(v.Error, "429") {
		t.Fatalf("error not shown: %+v", v)
	}
	if a.Briefing().Brief != nil {
		t.Fatal("failure cached")
	}
}

func TestRunBriefingInFlight(t *testing.T) {
	a, f := aiTestApp(t)
	f.answers = []string{goodBrief, goodBrief}
	f.gate = make(chan struct{})
	done := make(chan BriefingView)
	go func() { done <- a.RunBriefing() }()
	deadline := time.Now().Add(2 * time.Second)
	for !a.Briefing().Running && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if v := a.RunBriefing(); !v.Running {
		t.Fatalf("second click while running must not start another call: %+v", v)
	}
	close(f.gate)
	<-done
	if f.calls() != 1 {
		t.Fatalf("calls = %d", f.calls())
	}
}

func TestBriefingLockedWithoutHistory(t *testing.T) {
	a := ledgerTestApp(t)
	f := &fakeRunner{}
	a.ai.runner = f
	if v := a.Briefing(); !v.Locked {
		t.Fatal("not locked")
	}
	if v := a.RunBriefing(); !v.Locked || f.calls() != 0 {
		t.Fatalf("ran without history: %+v", v)
	}
}

var _ = stats.Query{}

func TestValidateAsk(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)
	ok := stats.Query{From: "2026-11-01", To: "2026-11-15", GroupBy: "project", Metric: "tokens"}
	bad := map[string][]stats.Query{
		"none":     {},
		"five":     {ok, ok, ok, ok, ok},
		"groupBy":  {{GroupBy: "files", Metric: "tokens"}},
		"metric":   {{GroupBy: "day", Metric: "secrets"}},
		"reversed": {{From: "2026-11-15", To: "2026-11-01", GroupBy: "day", Metric: "tokens"}},
		"too wide": {{From: "2024-01-01", To: "2026-11-15", GroupBy: "day", Metric: "tokens"}},
		"open":     {{GroupBy: "day", Metric: "tokens"}},
	}
	for name, qs := range bad {
		if validateAsk(qs, now) == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := validateAsk([]stats.Query{ok}, now); err != nil {
		t.Fatal(err)
	}
}

func TestAskStatsTwoCallsAggregatesOnly(t *testing.T) {
	a, f := aiTestApp(t)
	from, to := briefWeek(time.Now())
	f.answers = []string{
		fmt.Sprintf(`{"queries":[{"from":%q,"to":%q,"groupBy":"project","metric":"turns"}]}`, from, to),
		`{"answer":"You spent most turns on p."}`,
	}
	v := a.AskStats("  where did my turns go?  ")
	if v.Error != "" || v.Answer != "You spent most turns on p." || len(v.Results) != 1 || len(v.Results[0].Rows) != 1 {
		t.Fatalf("view %+v", v)
	}
	if f.calls() != 2 {
		t.Fatalf("calls %d", f.calls())
	}
	for _, p := range f.prompts {
		if strings.Contains(p, "TOP SECRET") || strings.Contains(p, "/Users/secret") {
			t.Fatalf("prompt leaks: %s", p)
		}
	}
	if !strings.Contains(f.prompts[1], `"value"`) {
		t.Fatalf("second prompt lacks the result rows: %s", f.prompts[1])
	}
}

func TestAskStatsRejectsBeforeRunning(t *testing.T) {
	a, f := aiTestApp(t)
	f.answers = []string{`{"queries":[{"groupBy":"files","metric":"tokens","from":"2026-11-01","to":"2026-11-02"}]}`}
	if v := a.AskStats("q"); v.Error == "" || f.calls() != 1 {
		t.Fatalf("invalid query ran: %+v calls=%d", v, f.calls())
	}
	if v := a.AskStats("   "); v.Error == "" || f.calls() != 1 {
		t.Fatalf("empty question called the model: %+v", v)
	}
	b := ledgerTestApp(t)
	b.ai.runner = f
	if v := b.AskStats("q"); v.Error == "" || f.calls() != 1 {
		t.Fatalf("ran without history: %+v", v)
	}
}

func TestWrappedTitleRules(t *testing.T) {
	cases := []struct {
		v    WrappedView
		want string
	}{
		{WrappedView{Commits: 1200, TopShare: 0.9, NightShare: 0.5}, "Shipwright"},
		{WrappedView{Commits: 10, TopShare: 0.7, TopProjects: []stats.Row{{Label: "vat"}}}, "Monk of vat"},
		{WrappedView{Commits: 10, TopShare: 0.3, NightShare: 0.3}, "Night Owl"},
		{WrappedView{Commits: 10, TopShare: 0.3, NightShare: 0.1}, "Commander"},
	}
	for _, c := range cases {
		if got := wrappedTitle(c.v); got != c.want {
			t.Fatalf("%+v → %q, want %q", c.v, got, c.want)
		}
	}
}

func TestWrappedYear(t *testing.T) {
	a, f := aiTestApp(t)
	y := time.Now().AddDate(0, 0, -7).Year()
	v := a.Wrapped(y)
	if v.Empty || v.Commits != 1 || len(v.TopProjects) != 1 || v.TopProjects[0].Label != "p" || v.Title == "" || len(v.Years) == 0 {
		t.Fatalf("wrapped %+v", v)
	}
	if e := a.Wrapped(1999); !e.Empty {
		t.Fatalf("empty year not empty: %+v", e)
	}
	if f.calls() != 0 {
		t.Fatal("Wrapped called a model")
	}
	if l := ledgerTestApp(t).Wrapped(y); !l.Locked {
		t.Fatal("not locked without history")
	}
}

func TestSaveWrappedPNGRejectsNonPNG(t *testing.T) {
	a := ledgerTestApp(t)
	for _, u := range []string{"", "data:text/html;base64,PGh0bWw+", "data:image/png;base64,PGh0bWw+", "data:image/png;base64,!!!"} {
		if _, err := a.SaveWrappedPNG(u); err == nil {
			t.Fatalf("accepted %q", u)
		}
	}
}

func TestProjectSessionsRangeAndTitles(t *testing.T) {
	a, _ := aiTestApp(t)
	key := repoid.KeyOf("path:/Users/secret/very-private-root")
	from, to := briefWeek(time.Now())
	rows := a.ProjectSessions(key, from, to)
	if len(rows) != 1 || rows[0].ID != "s1" || rows[0].Title != "TOP SECRET TITLE" || rows[0].ActiveMs != 60000 {
		t.Fatalf("rows %+v", rows)
	}
	if out := a.ProjectSessions(key, "1999-01-01", "1999-12-31"); len(out) != 0 {
		t.Fatalf("range not applied: %+v", out)
	}
	st := a.ledger.currentStore()
	st.SetConsent(store.Consent{Enabled: true, StoreTitles: false})
	if rows := a.ProjectSessions(key, from, to); len(rows) != 1 || rows[0].Title != "" {
		t.Fatalf("title shown without consent: %+v", rows)
	}
}

// Review fix: day-grouped answers kept only the oldest 50 days.
func TestAskRowsKeepEveryDay(t *testing.T) {
	var rows []stats.Row
	for i := 0; i < 90; i++ {
		rows = append(rows, stats.Row{Key: fmt.Sprintf("d%02d", i), Value: float64(i)})
	}
	kept, cut := askRows(stats.Query{GroupBy: "day"}, rows)
	if len(kept) != 90 || cut {
		t.Fatalf("day rows %d cut=%v", len(kept), cut)
	}
	var many []stats.Row
	for i := 0; i < 80; i++ {
		many = append(many, stats.Row{Key: fmt.Sprint(i), Value: float64(80 - i)})
	}
	kept, cut = askRows(stats.Query{GroupBy: "model"}, many)
	if len(kept) != 50 || !cut || kept[0].Value != 80 {
		t.Fatalf("model rows %d cut=%v", len(kept), cut)
	}
}

// Review fix: the Ask disclosure must be visible before anything is sent.
func TestBriefingCarriesAskDisclosure(t *testing.T) {
	a, f := aiTestApp(t)
	if v := a.Briefing(); !strings.Contains(v.AskDisclosure, "your question") || f.calls() != 0 {
		t.Fatalf("view %+v", v)
	}
}
