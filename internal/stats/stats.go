// Package stats is Al Asturlab's one aggregation path: every tile, chart and
// export reads the ledger through Engine, so numbers cannot disagree.
package stats

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/rates"
)

type Source interface {
	Gen() uint64
	Projects() []store.ProjectInfo
	Sessions(key string) []store.SessionSummary
}

type Query struct {
	From    string `json:"from"` // YYYY-MM-DD local, inclusive; "" = open
	To      string `json:"to"`
	GroupBy string `json:"groupBy"`
	Split   string `json:"split,omitempty"`
	Metric  string `json:"metric"`
	Project string `json:"project,omitempty"`
	Model   string `json:"model,omitempty"`
}

type Row struct {
	Key      string  `json:"key"`
	Label    string  `json:"label,omitempty"`
	Split    string  `json:"split,omitempty"`
	Value    float64 `json:"value"`
	Unpriced int64   `json:"unpriced,omitempty"`
}

type Result struct {
	Rows     []Row  `json:"rows"`
	PricedAt string `json:"pricedAt"`
	Stale    bool   `json:"stale"`
}

type Tiles struct {
	Tokens         int64   `json:"tokens"`
	ActiveMs       int64   `json:"activeMs"`
	Commits        int64   `json:"commits"`
	CostUSD        float64 `json:"costUSD"`
	UnpricedTokens int64   `json:"unpricedTokens"`
	PlanUSD        float64 `json:"planUSD"`
	PlanX          float64 `json:"planX"`
	CacheShare     float64 `json:"cacheShare"`
	CacheSavedUSD  float64 `json:"cacheSavedUSD"`
	PricedAt       string  `json:"pricedAt"`
	Stale          bool    `json:"stale"`
}

type Overview struct {
	Cur  Tiles `json:"cur"`
	Prev Tiles `json:"prev"`
}

type ModelRow struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	Tokens      int64   `json:"tokens"`
	Turns       float64 `json:"turns"`
	Errors      float64 `json:"errors"`
	Compactions float64 `json:"compactions"`
	TestRuns    float64 `json:"testRuns"`
	TestFails   float64 `json:"testFails"`
	Commits     float64 `json:"commits"`
	Lines       float64 `json:"lines"`
	CostUSD     float64 `json:"costUSD"`
	Priced      bool    `json:"priced"`
}

type DayRow struct {
	Day        string            `json:"day"`
	ProjectKey string            `json:"projectKey"`
	Project    string            `json:"project"`
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Tokens     store.TokenCounts `json:"tokens"`
	CostUSD    float64           `json:"costUSD"`
	Priced     bool              `json:"priced"`
	ActiveMs   int64             `json:"activeMs"`
	Turns      int64             `json:"turns"`
	Commits    int64             `json:"commits"`
}

const daysPerMonth = 30.44

type Engine struct {
	mu      sync.Mutex
	src     Source
	book    *rates.Book
	catalog func(string) string
	now     func() time.Time
	built   bool
	gen     uint64
	facts   []fact
	labels  map[string]string
}

func New(src Source, book *rates.Book, catalog func(string) string, now func() time.Time) *Engine {
	return &Engine{src: src, book: book, catalog: catalog, now: now}
}

// SetBook swaps rates/plans; costs are computed per query, so facts stay cached.
func (e *Engine) SetBook(b *rates.Book) {
	e.mu.Lock()
	e.book = b
	e.mu.Unlock()
}

// load returns the cached facts, rebuilding them when the ledger changed.
func (e *Engine) load() ([]fact, map[string]string, *rates.Book) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if g := e.src.Gen(); !e.built || g != e.gen {
		e.facts, e.labels = flatten(e.src, e.catalog)
		e.gen, e.built = g, true
	}
	return e.facts, e.labels, e.book
}

var (
	groupBys = map[string]bool{"day": true, "provider": true, "model": true, "project": true, "tool": true, "weekday-hour": true}
	metrics  = map[string]bool{"tokens": true, "cost": true, "activeMs": true, "turns": true, "commits": true, "errors": true,
		"compactions": true, "testRuns": true, "testFails": true, "lines": true, "subagents": true, "sessions": true, "cacheRead": true}
	hourMetrics = map[string]bool{"turns": true, "activeMs": true, "tokens": true}
)

func validDay(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (q Query) validate() error {
	switch {
	case !groupBys[q.GroupBy]:
		return fmt.Errorf("groupBy %q not one of day|provider|model|project|tool|weekday-hour", q.GroupBy)
	case !metrics[q.Metric]:
		return fmt.Errorf("metric %q unknown", q.Metric)
	case q.Split != "" && q.Split != "model" && q.Split != "provider":
		return fmt.Errorf("split %q not model|provider", q.Split)
	case q.Split != "" && q.GroupBy != "day":
		return fmt.Errorf("split only with groupBy day")
	case q.GroupBy == "tool" && q.Metric != "turns":
		return fmt.Errorf("groupBy tool counts tool calls: metric must be turns")
	case q.GroupBy == "weekday-hour" && !hourMetrics[q.Metric]:
		return fmt.Errorf("groupBy weekday-hour supports turns|activeMs|tokens")
	case !validDay(q.From) || !validDay(q.To):
		return fmt.Errorf("from/to must be YYYY-MM-DD")
	case q.From != "" && q.To != "" && q.From > q.To:
		return fmt.Errorf("from %s after to %s", q.From, q.To)
	}
	return nil
}

func (q Query) keep(f *fact) bool {
	return (q.From == "" || f.day >= q.From) && (q.To == "" || f.day <= q.To) &&
		(q.Project == "" || f.project == q.Project) && (q.Model == "" || f.model == q.Model)
}

func priced(b *rates.Book, f *fact) (float64, bool) {
	r, ok := b.Rate(f.model, f.provider)
	if !ok {
		return 0, false
	}
	return r.Cost(f.tok), true
}

func counter(f *fact, metric string) float64 {
	c := f.c
	var v int64
	switch metric {
	case "activeMs":
		v = c.ActiveMs
	case "turns":
		v = c.Turns
	case "commits":
		v = c.Commits
	case "errors":
		v = c.Errors
	case "compactions":
		v = c.Compactions
	case "testRuns":
		v = c.TestRuns
	case "testFails":
		v = c.TestFails
	case "lines":
		v = c.LinesAdded + c.LinesRemoved
	case "subagents":
		v = c.Subagents
	}
	return float64(v) * f.share
}

func (e *Engine) meta(b *rates.Book) (string, bool) {
	return rates.PricedAt().Format("2006-01-02"), rates.Stale(e.now())
}

// Run answers one query.
func (e *Engine) Run(q Query) (Result, error) {
	if err := q.validate(); err != nil {
		return Result{}, err
	}
	facts, labels, book := e.load()
	type k struct{ key, split string }
	vals := map[k]*Row{}
	sess := map[k]map[string]bool{}
	add := func(key, split string, v float64, unpriced int64) {
		kk := k{key, split}
		r := vals[kk]
		if r == nil {
			r = &Row{Key: key, Split: split}
			if q.GroupBy == "project" {
				r.Label = labels[key]
			}
			vals[kk] = r
		}
		r.Value += v
		r.Unpriced += unpriced
	}
	for i := range facts {
		f := &facts[i]
		if !q.keep(f) {
			continue
		}
		switch q.GroupBy {
		case "tool":
			for name, n := range f.c.Tools {
				add(name, "", float64(n)*f.share, 0)
			}
			continue
		case "weekday-hour":
			wd := weekday(f.day)
			for h, x := range f.c.Hours {
				v := map[string]float64{"turns": float64(x.Turns), "activeMs": float64(x.ActiveMs), "tokens": float64(x.Tokens)}[q.Metric] * f.share
				if v != 0 {
					add(fmt.Sprintf("%d-%d", wd, h), "", v, 0)
				}
			}
			continue
		}
		key := map[string]string{"day": f.day, "provider": f.provider, "model": f.model, "project": f.project}[q.GroupBy]
		split := map[string]string{"": "", "model": f.model, "provider": f.provider}[q.Split]
		switch q.Metric {
		case "tokens":
			add(key, split, float64(f.tok.Total()), 0)
		case "cacheRead":
			add(key, split, float64(f.tok.CacheRead), 0)
		case "cost":
			if c, ok := priced(book, f); ok {
				add(key, split, c, 0)
			} else {
				add(key, split, 0, f.tok.Total())
			}
		case "sessions":
			kk := k{key, split}
			if sess[kk] == nil {
				sess[kk] = map[string]bool{}
			}
			if !sess[kk][f.session] {
				sess[kk][f.session] = true
				add(key, split, 1, 0)
			}
		default:
			add(key, split, counter(f, q.Metric), 0)
		}
	}
	rows := make([]Row, 0, len(vals))
	for _, r := range vals {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if q.GroupBy == "day" && rows[i].Key != rows[j].Key {
			return rows[i].Key < rows[j].Key
		}
		if rows[i].Value != rows[j].Value {
			return rows[i].Value > rows[j].Value
		}
		return rows[i].Key+rows[i].Split < rows[j].Key+rows[j].Split
	})
	at, stale := e.meta(book)
	return Result{Rows: rows, PricedAt: at, Stale: stale}, nil
}

func weekday(day string) int {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return 0
	}
	return (int(t.Weekday()) + 6) % 7 // Mon = 0
}

func spanDays(from, to string) int {
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || b.Before(a) {
		return 0
	}
	return int(math.Round(b.Sub(a).Hours()/24)) + 1
}

func (e *Engine) tiles(facts []fact, book *rates.Book, from, to string) Tiles {
	var t Tiles
	t.PricedAt, t.Stale = e.meta(book)
	q := Query{From: from, To: to}
	first := ""
	var in, read, write int64
	planCost := 0.0
	plans := book.Settings().Plans
	for i := range facts {
		f := &facts[i]
		if !q.keep(f) {
			continue
		}
		if first == "" || f.day < first {
			first = f.day
		}
		t.Tokens += f.tok.Total()
		t.ActiveMs += int64(math.Round(counter(f, "activeMs")))
		in, read, write = in+f.tok.In, read+f.tok.CacheRead, write+f.tok.CacheWrite
		if c, ok := priced(book, f); ok {
			t.CostUSD += c
			if _, has := plans[f.provider]; has {
				planCost += c
			}
			r, _ := book.Rate(f.model, f.provider)
			t.CacheSavedUSD += float64(f.tok.CacheRead) * (r.In - r.CacheRead) / 1e6
		} else {
			t.UnpricedTokens += f.tok.Total()
		}
	}
	// Commits are whole numbers per day; sum them unsplit.
	seen := map[*store.Counters]bool{}
	for i := range facts {
		f := &facts[i]
		if q.keep(f) && !seen[f.c] {
			seen[f.c] = true
			t.Commits += f.c.Commits
		}
	}
	if d := in + read + write; d > 0 {
		t.CacheShare = float64(read) / float64(d)
	}
	if from == "" {
		from = first
	}
	if to == "" {
		to = e.now().Format("2006-01-02")
	}
	if n := spanDays(from, to); n > 0 {
		for _, p := range plans {
			t.PlanUSD += p.MonthlyUSD * float64(n) / daysPerMonth
		}
	}
	if t.PlanUSD > 0 {
		t.PlanX = planCost / t.PlanUSD
	}
	return t
}

// Overview: tiles for [from,to] and for the equal-length period just before.
func (e *Engine) Overview(from, to string) Overview {
	facts, _, book := e.load()
	o := Overview{Cur: e.tiles(facts, book, from, to)}
	if n := spanDays(from, to); n > 0 {
		a, _ := time.Parse("2006-01-02", from)
		o.Prev = e.tiles(facts, book, a.AddDate(0, 0, -n).Format("2006-01-02"), a.AddDate(0, 0, -1).Format("2006-01-02"))
	} else {
		o.Prev.PricedAt, o.Prev.Stale = e.meta(book)
	}
	return o
}

// Models compares models over a range (optionally one project), by tokens desc.
func (e *Engine) Models(from, to, project string) []ModelRow {
	facts, _, book := e.load()
	q := Query{From: from, To: to, Project: project}
	by := map[string]*ModelRow{}
	for i := range facts {
		f := &facts[i]
		if !q.keep(f) {
			continue
		}
		m := by[f.model]
		if m == nil {
			_, ok := book.Rate(f.model, f.provider)
			m = &ModelRow{Model: f.model, Provider: f.provider, Priced: ok}
			by[f.model] = m
		}
		m.Tokens += f.tok.Total()
		m.Turns += counter(f, "turns")
		m.Errors += counter(f, "errors")
		m.Compactions += counter(f, "compactions")
		m.TestRuns += counter(f, "testRuns")
		m.TestFails += counter(f, "testFails")
		m.Commits += counter(f, "commits")
		m.Lines += counter(f, "lines")
		if c, ok := priced(book, f); ok {
			m.CostUSD += c
		}
	}
	out := make([]ModelRow, 0, len(by))
	for _, m := range by {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens != out[j].Tokens {
			return out[i].Tokens > out[j].Tokens
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// Days is the export table: one row per day × project × model.
func (e *Engine) Days(from, to string) []DayRow {
	facts, labels, book := e.load()
	q := Query{From: from, To: to}
	type k struct{ d, p, m string }
	by := map[k]*DayRow{}
	for i := range facts {
		f := &facts[i]
		if !q.keep(f) {
			continue
		}
		kk := k{f.day, f.project, f.model}
		r := by[kk]
		if r == nil {
			_, ok := book.Rate(f.model, f.provider)
			r = &DayRow{Day: f.day, ProjectKey: f.project, Project: labels[f.project], Provider: f.provider, Model: f.model, Priced: ok}
			by[kk] = r
		}
		r.Tokens.In += f.tok.In
		r.Tokens.Out += f.tok.Out
		r.Tokens.CacheRead += f.tok.CacheRead
		r.Tokens.CacheWrite += f.tok.CacheWrite
		if c, ok := priced(book, f); ok {
			r.CostUSD += c
		}
		r.ActiveMs += int64(math.Round(counter(f, "activeMs")))
		if f.major { // whole counters stay whole: the day's biggest model carries them
			r.Turns += f.c.Turns
			r.Commits += f.c.Commits
		}
	}
	out := make([]DayRow, 0, len(by))
	for _, r := range by {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Project != b.Project {
			return a.Project < b.Project
		}
		return a.Model < b.Model
	})
	return out
}
