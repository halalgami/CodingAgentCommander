package main

import (
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/rates"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
	"github.com/halalgami/CodingAgentCommander/internal/stats"
)

func applyTokens(st *store.Store, root, sid, model string, in int64) string {
	id := repoid.ID{Key: repoid.KeyOf("path:" + root), Identity: "path:" + root, Root: root, Label: "p"}
	st.Apply(id, "/t/"+sid+".jsonl", store.Cursor{SessionID: sid}, false, []parse.Event{
		{Kind: parse.Assistant, At: time.Now().UnixMilli(), MessageID: sid, Model: model, HasUsage: true, Usage: parse.Usage{In: in}},
	}, 0)
	return id.Key
}

func today() string { return time.Now().Format("2006-01-02") }

func TestStatsLockedWithoutHistory(t *testing.T) {
	a := ledgerTestApp(t)
	if v := a.StatsOverview(today(), today()); !v.Locked {
		t.Fatalf("history off: %+v", v)
	}
	if _, err := a.Stats(stats.Query{GroupBy: "day", Metric: "tokens"}); err == nil {
		t.Fatal("Stats without history should error")
	}
}

func TestStatsRunsOnStore(t *testing.T) {
	a := ledgerTestApp(t)
	st := attachTestStore(t, a)
	applyTokens(st, "/p1", "s1", "claude-opus-5-5", 1_000_000)
	res, err := a.Stats(stats.Query{GroupBy: "model", Metric: "tokens"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0].Key != "claude-opus-5-5" || res.Rows[0].Value != 1e6 {
		t.Fatalf("%+v %v", res, err)
	}
	// a later write is visible (cache keyed on store Gen)
	applyTokens(st, "/p1", "s2", "claude-opus-5-5", 1)
	res, _ = a.Stats(stats.Query{GroupBy: "model", Metric: "tokens"})
	if res.Rows[0].Value != 1e6+1 {
		t.Fatalf("stale: %+v", res)
	}
}

func TestSetAsturlabSettingsChangesCost(t *testing.T) {
	a := ledgerTestApp(t)
	st := attachTestStore(t, a)
	applyTokens(st, "/p1", "s1", "glm-5.3", 1_000_000)
	if v := a.StatsOverview(today(), today()); v.Overview.Cur.CostUSD != 0 || v.Overview.Cur.UnpricedTokens != 1e6 {
		t.Fatalf("before: %+v", v.Overview.Cur)
	}
	s := rates.Settings{Overrides: map[string]rates.Rate{"glm-5.3": {In: 2}}}
	if err := a.SetAsturlabSettings(s); err != nil {
		t.Fatal(err)
	}
	if got := a.AsturlabSettings(); got.Overrides["glm-5.3"].In != 2 {
		t.Fatalf("not persisted: %+v", got)
	}
	if v := a.StatsOverview(today(), today()); v.Overview.Cur.CostUSD != 2 {
		t.Fatalf("after: %+v", v.Overview.Cur)
	}
}

func TestExportCSVHeaderAndRows(t *testing.T) {
	rows := []stats.DayRow{
		{Day: "2026-11-11", Project: "Ayina, RDP", Provider: "anthropic", Model: "claude-opus-5-5", Tokens: store.TokenCounts{In: 5}, CostUSD: 0.5, Priced: true, Turns: 2},
		{Day: "2026-11-11", Project: "x", Provider: "ollama-cloud", Model: "glm-5.3", Tokens: store.TokenCounts{In: 7}},
	}
	b, err := encodeExport("days", "csv", rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] != "day,project,provider,model,in,out,cacheRead,cacheWrite,costUSD,priced,activeMs,turns,commits" {
		t.Fatalf("header %q", lines[0])
	}
	if lines[1] != `2026-11-11,"Ayina, RDP",anthropic,claude-opus-5-5,5,0,0,0,0.500000,true,0,2,0` {
		t.Fatalf("row1 %q", lines[1])
	}
	if !strings.Contains(lines[2], ",7,0,0,0,,false,") {
		t.Fatalf("unpriced cost must be empty: %q", lines[2])
	}
	if b, err := encodeExport("chart", "json", nil, []stats.Row{{Key: "a", Value: 1}}); err != nil || !strings.Contains(string(b), `"key": "a"`) {
		t.Fatalf("json %s %v", b, err)
	}
	if _, err := encodeExport("chart", "xml", nil, nil); err == nil {
		t.Fatal("xml accepted")
	}
}
