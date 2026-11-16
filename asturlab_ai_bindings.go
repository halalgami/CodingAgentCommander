package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/askai"
	"github.com/halalgami/CodingAgentCommander/internal/stats"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Al Asturlab AI features (spec §7). Every model call here starts from a user
// click; nothing in this file runs on a timer, at startup or on view open.

type aiState struct {
	mu       sync.Mutex
	runner   askai.Runner // nil → askai.Claude{}
	briefing bool         // a briefing call is in flight
	asking   bool         // an ask call is in flight
}

func (a *App) aiRunner() askai.Runner {
	a.ai.mu.Lock()
	defer a.ai.mu.Unlock()
	if a.ai.runner == nil {
		a.ai.runner = askai.Claude{}
	}
	return a.ai.runner
}

func (a *App) aiModel() string {
	if m := strings.TrimSpace(a.statsBook().Settings().AIModel); m != "" {
		return m
	}
	return "haiku"
}

func aiCtx(a *App) context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// ---- weekly briefing ----

// briefWeek is the last complete Monday–Sunday week before now (local).
func briefWeek(now time.Time) (from, to string) {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	mon := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7)) // this week's Monday
	start := mon.AddDate(0, 0, -7)
	return start.Format("2006-01-02"), start.AddDate(0, 0, 6).Format("2006-01-02")
}

type namedValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// BriefFacts is everything the briefing model sees: totals and labels only.
type BriefFacts struct {
	From         string       `json:"from"`
	To           string       `json:"to"`
	ThisWeek     stats.Tiles  `json:"thisWeek"`
	PreviousWeek stats.Tiles  `json:"previousWeek"`
	TopProjects  []namedValue `json:"topProjectsByTokens"`
	Models       []namedValue `json:"modelsByTokens"`
	BusiestHour  string       `json:"busiestWeekdayHour,omitempty"`
	BusiestDay   *namedValue  `json:"busiestDayByTokens,omitempty"`
}

var weekdayNames = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

func topRows(e *stats.Engine, q stats.Query, n int, label bool) []namedValue {
	res, err := e.Run(q)
	if err != nil {
		return []namedValue{}
	}
	out := []namedValue{}
	for i, r := range res.Rows {
		if i == n {
			break
		}
		name := r.Key
		if label {
			name = r.Label
		}
		out = append(out, namedValue{name, r.Value})
	}
	return out
}

func briefFacts(e *stats.Engine, from, to string) BriefFacts {
	ov := e.Overview(from, to)
	f := BriefFacts{From: from, To: to, ThisWeek: ov.Cur, PreviousWeek: ov.Prev}
	f.TopProjects = topRows(e, stats.Query{From: from, To: to, GroupBy: "project", Metric: "tokens"}, 5, true)
	f.Models = topRows(e, stats.Query{From: from, To: to, GroupBy: "model", Metric: "tokens"}, 6, false)
	if h := topRows(e, stats.Query{From: from, To: to, GroupBy: "weekday-hour", Metric: "turns"}, 1, false); len(h) == 1 {
		wd, hr, _ := strings.Cut(h[0].Name, "-")
		w, _ := strconv.Atoi(wd)
		if w >= 0 && w < 7 {
			f.BusiestHour = fmt.Sprintf("%s %s:00", weekdayNames[w], hr)
		}
	}
	days := topRows(e, stats.Query{From: from, To: to, GroupBy: "day", Metric: "tokens"}, 7, false)
	sort.Slice(days, func(i, j int) bool { return days[i].Value > days[j].Value })
	if len(days) > 0 {
		f.BusiestDay = &days[0]
	}
	return f
}

type BriefPoint struct {
	Text  string `json:"text"`
	Chart string `json:"chart"`
}

type Brief struct {
	Week     string       `json:"week"`
	Headline string       `json:"headline"`
	Points   []BriefPoint `json:"points"`
	Model    string       `json:"model"`
	At       string       `json:"at"`
}

type BriefingView struct {
	Locked     bool   `json:"locked"`
	Week       string `json:"week"`
	Brief      *Brief `json:"brief"`
	Running    bool   `json:"running"`
	Disclosure string `json:"disclosure"`
	// AskDisclosure is shown under the Ask box before anything is sent.
	AskDisclosure string `json:"askDisclosure"`
	Error         string `json:"error,omitempty"`
}

var briefCharts = map[string]bool{"": true, "spend": true, "treemap": true, "punch": true, "calendar": true, "models": true, "tools": true}

const briefSchema = `{"type":"object","properties":{"headline":{"type":"string"},"points":{"type":"array","maxItems":5,"items":{"type":"object","properties":{"text":{"type":"string"},"chart":{"type":"string","enum":["","spend","treemap","punch","calendar","models","tools"]}},"required":["text","chart"]}}},"required":["headline","points"]}`

const briefPrompt = `You write a short weekly briefing for a developer about how they used AI coding agents last week.
Below are the week's totals as JSON (tokens, active milliseconds, commits, API-equivalent cost estimate in USD,
cache share, top projects, models, busiest hour and day), plus the previous week for comparison.
Write one headline (max 12 words) and 3-5 points (max 25 words each). Compare with the previous week where it
matters, name projects and models by the names given, round numbers, call dollar figures estimates.
Tag each point with the chart that shows it: spend (tokens/cost over time), treemap (projects), punch
(time of day), calendar (days), models, tools; or "" if none. Do not invent numbers.

DATA:
`

func (a *App) briefPath() string {
	return filepath.Join(filepath.Dir(configPath()), "asturlab-briefing.json")
}

func (a *App) loadBriefs() map[string]Brief {
	m := map[string]Brief{}
	if b, err := os.ReadFile(a.briefPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (a *App) saveBrief(b Brief) error {
	m := a.loadBriefs()
	m[b.Week] = b
	weeks := make([]string, 0, len(m))
	for w := range m {
		weeks = append(weeks, w)
	}
	sort.Strings(weeks)
	for len(weeks) > 8 { // keep two months
		delete(m, weeks[0])
		weeks = weeks[1:]
	}
	body, _ := json.MarshalIndent(m, "", "  ")
	return writeFileAtomic(a.briefPath(), body)
}

func (a *App) briefDisclosure(from, to string) string {
	return fmt.Sprintf("Sends %s to %s totals (tokens, hours, commits, cost estimate, project names, model names) to %s. No titles, paths or conversation text.", from, to, a.aiModel())
}

// Briefing shows the cached briefing for last week, if any. Never calls a model.
func (a *App) Briefing() BriefingView {
	from, to := briefWeek(time.Now())
	v := BriefingView{Week: from, Disclosure: a.briefDisclosure(from, to), AskDisclosure: a.askDisclosure()}
	if a.engine() == nil {
		v.Locked = true
		return v
	}
	a.ai.mu.Lock()
	v.Running = a.ai.briefing
	a.ai.mu.Unlock()
	if b, ok := a.loadBriefs()[from]; ok {
		v.Brief = &b
	}
	return v
}

// RunBriefing is the click: one model call for last week, cached on success.
func (a *App) RunBriefing() BriefingView {
	v := a.Briefing()
	if v.Locked || v.Running {
		return v
	}
	a.ai.mu.Lock()
	if a.ai.briefing {
		a.ai.mu.Unlock()
		v.Running = true
		return v
	}
	a.ai.briefing = true
	a.ai.mu.Unlock()
	defer func() { a.ai.mu.Lock(); a.ai.briefing = false; a.ai.mu.Unlock() }()

	from, to := briefWeek(time.Now())
	facts, _ := json.Marshal(briefFacts(a.engine(), from, to))
	model := a.aiModel()
	raw, err := a.aiRunner().Run(aiCtx(a), model, briefPrompt+string(facts), briefSchema)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	var b Brief
	if err := json.Unmarshal(raw, &b); err != nil || strings.TrimSpace(b.Headline) == "" || len(b.Points) == 0 {
		v.Error = "the model's answer was incomplete; try again"
		return v
	}
	for i := range b.Points {
		if !briefCharts[b.Points[i].Chart] {
			b.Points[i].Chart = ""
		}
	}
	b.Week, b.Model, b.At = from, model, time.Now().Format(time.RFC3339)
	if err := a.saveBrief(b); err != nil {
		v.Error = "could not save the briefing: " + err.Error()
	}
	v.Brief = &b
	return v
}

var errAINoHistory = errors.New("turn on history to use this")

// ---- Ask your stats ----

type AskView struct {
	Question   string         `json:"question"`
	Queries    []stats.Query  `json:"queries"`
	Results    []stats.Result `json:"results"`
	Answer     string         `json:"answer"`
	Error      string         `json:"error,omitempty"`
	Disclosure string         `json:"disclosure"`
}

// askRows bounds what one answer carries. Day rows (at most 400 days × a few
// splits) are all kept: they come oldest first, so a cut would drop the recent
// days. Other groupings come biggest first and keep the top 50.
func askRows(q stats.Query, rows []stats.Row) ([]stats.Row, bool) {
	limit := 50
	if q.GroupBy == "day" {
		limit = 2000
	}
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

const maxAskQueries = 4
const maxAskDays = 400

// validateAsk vets model-written queries before anything runs.
func validateAsk(qs []stats.Query, now time.Time) error {
	if len(qs) == 0 {
		return errors.New("the model asked for no data")
	}
	if len(qs) > maxAskQueries {
		return fmt.Errorf("the model asked for %d queries (max %d)", len(qs), maxAskQueries)
	}
	for _, q := range qs {
		if err := stats.Validate(q); err != nil {
			return fmt.Errorf("the model wrote an invalid query: %v", err)
		}
		if q.From == "" || q.To == "" {
			return errors.New("the model wrote a query without a date range")
		}
		f, _ := time.ParseInLocation("2006-01-02", q.From, time.Local)
		t, _ := time.ParseInLocation("2006-01-02", q.To, time.Local)
		if t.Sub(f) > maxAskDays*24*time.Hour {
			return fmt.Errorf("the model asked for more than %d days", maxAskDays)
		}
	}
	return nil
}

const askSchema1 = `{"type":"object","properties":{"queries":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"},"groupBy":{"type":"string","enum":["day","provider","model","project","tool","weekday-hour"]},"split":{"type":"string","enum":["","model","provider"]},"metric":{"type":"string","enum":["tokens","cost","activeMs","turns","commits","errors","compactions","testRuns","testFails","lines","subagents","sessions","cacheRead"]},"project":{"type":"string"},"model":{"type":"string"}},"required":["from","to","groupBy","metric"]}}},"required":["queries"]}`

const askSchema2 = `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`

const askPrompt1 = `You translate a developer's question about their AI coding-agent usage into 1-4 data queries.
Each query: from/to (YYYY-MM-DD, inclusive, at most 400 days), groupBy (day|provider|model|project|tool|weekday-hour),
metric (tokens|cost|activeMs|turns|commits|errors|compactions|testRuns|testFails|lines|subagents|sessions|cacheRead),
optional split (model|provider, only with groupBy day), optional project (a project KEY from the list) and model (a model id).
Rules: groupBy tool needs metric turns; weekday-hour supports turns|activeMs|tokens and keys are "<weekday 0=Mon>-<hour>".
cost is an API-equivalent estimate in USD; activeMs is milliseconds.
`

const askPrompt2 = `Answer the developer's question in 1-3 short sentences using ONLY the query results below.
Round numbers; call dollar amounts estimates; convert activeMs to hours. If the data cannot answer it, say so.
`

func (a *App) askDisclosure() string {
	return "Sends your question, project names and model names to " + a.aiModel() + ", then the resulting totals. No titles, paths or conversation text."
}

// AskStats is the Ask box's Enter: two model calls (question → queries,
// results → answer). The model never runs anything itself.
func (a *App) AskStats(question string) AskView {
	question = strings.TrimSpace(question)
	v := AskView{Question: question, Queries: []stats.Query{}, Results: []stats.Result{}, Disclosure: a.askDisclosure()}
	e := a.engine()
	switch {
	case e == nil:
		v.Error = errAINoHistory.Error()
		return v
	case question == "":
		v.Error = "type a question first"
		return v
	case len(question) > 500:
		v.Error = "keep the question under 500 characters"
		return v
	}
	a.ai.mu.Lock()
	if a.ai.asking {
		a.ai.mu.Unlock()
		v.Error = "still answering the last question"
		return v
	}
	a.ai.asking = true
	a.ai.mu.Unlock()
	defer func() { a.ai.mu.Lock(); a.ai.asking = false; a.ai.mu.Unlock() }()

	type proj struct{ Key, Name string }
	ctxInfo := struct {
		Today    string       `json:"today"`
		Projects []proj       `json:"projects"`
		Models   []namedValue `json:"models"`
		Question string       `json:"question"`
	}{Today: time.Now().Format("2006-01-02"), Projects: []proj{}, Question: question}
	if res, err := e.Run(stats.Query{GroupBy: "project", Metric: "tokens"}); err == nil {
		for i, r := range res.Rows {
			if i == 100 { // biggest 100 by tokens keeps the prompt bounded
				break
			}
			ctxInfo.Projects = append(ctxInfo.Projects, proj{r.Key, r.Label})
		}
	}
	ctxInfo.Models = topRows(e, stats.Query{GroupBy: "model", Metric: "tokens"}, 20, false)
	in1, _ := json.Marshal(ctxInfo)
	model, run := a.aiModel(), a.aiRunner()
	raw, err := run.Run(aiCtx(a), model, askPrompt1+"\nCONTEXT:\n"+string(in1), askSchema1)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	var plan struct {
		Queries []stats.Query `json:"queries"`
	}
	if json.Unmarshal(raw, &plan) != nil {
		v.Error = "the model's answer was not a query"
		return v
	}
	if err := validateAsk(plan.Queries, time.Now()); err != nil {
		v.Error = err.Error()
		return v
	}
	v.Queries = plan.Queries
	for _, q := range plan.Queries {
		res, err := e.Run(q)
		if err != nil {
			v.Error = err.Error()
			return v
		}
		res.Rows, _ = askRows(q, res.Rows)
		v.Results = append(v.Results, res)
	}
	in2, _ := json.Marshal(struct {
		Question string         `json:"question"`
		Queries  []stats.Query  `json:"queries"`
		Results  []stats.Result `json:"results"`
	}{question, v.Queries, v.Results})
	raw, err = run.Run(aiCtx(a), model, askPrompt2+"\nDATA:\n"+string(in2), askSchema2)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	var ans struct {
		Answer string `json:"answer"`
	}
	if json.Unmarshal(raw, &ans) != nil || strings.TrimSpace(ans.Answer) == "" {
		v.Error = "the model gave no answer; the data is shown below"
		return v
	}
	v.Answer = strings.TrimSpace(ans.Answer)
	return v
}

// ---- Commander Wrapped (no model call; the title is rule-based) ----

type WrappedView struct {
	Year         int         `json:"year"`
	Locked       bool        `json:"locked"`
	Empty        bool        `json:"empty"`
	Tokens       int64       `json:"tokens"`
	ActiveMs     int64       `json:"activeMs"`
	Commits      int64       `json:"commits"`
	CostUSD      float64     `json:"costUSD"`
	PricedAt     string      `json:"pricedAt"`
	TopProjects  []stats.Row `json:"topProjects"`
	BiggestDay   stats.Row   `json:"biggestDay"`
	Models       []stats.Row `json:"models"`
	Level        int         `json:"level"`
	Achievements int         `json:"achievements"`
	TopShare     float64     `json:"topShare"`   // top project's share of tokens
	NightShare   float64     `json:"nightShare"` // share of turns between 00:00 and 05:00
	Title        string      `json:"title"`
	Years        []int       `json:"years"`
}

func wrappedTitle(v WrappedView) string {
	switch {
	case v.Commits >= 1000:
		return "Shipwright"
	case v.TopShare >= 0.6 && len(v.TopProjects) > 0:
		return "Monk of " + v.TopProjects[0].Label
	case v.NightShare >= 0.25:
		return "Night Owl"
	}
	return "Commander"
}

func (a *App) Wrapped(year int) WrappedView {
	v := WrappedView{Year: year, TopProjects: []stats.Row{}, Models: []stats.Row{}, Years: []int{}}
	e := a.engine()
	if e == nil {
		v.Locked = true
		return v
	}
	if days, err := e.Run(stats.Query{GroupBy: "day", Metric: "tokens"}); err == nil {
		seen := map[int]bool{}
		for _, r := range days.Rows {
			if y, err := strconv.Atoi(r.Key[:min(4, len(r.Key))]); err == nil && !seen[y] {
				seen[y] = true
				v.Years = append(v.Years, y)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(v.Years)))
	}
	from, to := fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-12-31", year)
	ov := e.Overview(from, to).Cur
	v.Tokens, v.ActiveMs, v.Commits, v.CostUSD, v.PricedAt = ov.Tokens, ov.ActiveMs, ov.Commits, ov.CostUSD, ov.PricedAt
	if v.Tokens == 0 && v.ActiveMs == 0 {
		v.Empty = true
		return v
	}
	run := func(q stats.Query, n int) []stats.Row {
		q.From, q.To = from, to
		res, err := e.Run(q)
		if err != nil {
			return []stats.Row{}
		}
		if len(res.Rows) > n {
			res.Rows = res.Rows[:n]
		}
		return res.Rows
	}
	all := run(stats.Query{GroupBy: "project", Metric: "tokens"}, 1000)
	v.TopProjects = all[:min(5, len(all))]
	if len(all) > 0 && v.Tokens > 0 {
		v.TopShare = all[0].Value / float64(v.Tokens)
	}
	v.Models = run(stats.Query{GroupBy: "model", Metric: "tokens"}, 6)
	if days := run(stats.Query{GroupBy: "day", Metric: "tokens"}, 400); len(days) > 0 {
		best := days[0]
		for _, d := range days {
			if d.Value > best.Value {
				best = d
			}
		}
		v.BiggestDay = best
	}
	var night, turns float64
	for _, r := range run(stats.Query{GroupBy: "weekday-hour", Metric: "turns"}, 200) {
		_, h, _ := strings.Cut(r.Key, "-")
		turns += r.Value
		if hr, _ := strconv.Atoi(h); hr < 5 {
			night += r.Value
		}
	}
	if turns > 0 {
		v.NightShare = night / turns
	}
	p := a.Progress()
	v.Level = p.Commander.Level
	v.Achievements = len(p.Commander.Unlocks)
	for _, pr := range p.Projects {
		v.Achievements += len(pr.Unlocks)
	}
	v.Title = wrappedTitle(v)
	return v
}

// SaveWrappedPNG writes the rendered Wrapped image where the user picks.
// "" when cancelled.
func (a *App) SaveWrappedPNG(dataURL string) (string, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return "", errors.New("not a PNG image")
	}
	body, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil {
		return "", errors.New("not a PNG image")
	}
	if len(body) > 20<<20 || !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
		return "", errors.New("not a PNG image")
	}
	if a.ctx == nil {
		return "", errors.New("no window")
	}
	dest, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{Title: "Save Commander Wrapped", DefaultFilename: fmt.Sprintf("commander-wrapped-%s.png", time.Now().Format("2006"))})
	if err != nil || dest == "" {
		return "", err
	}
	return dest, os.WriteFile(dest, body, 0o644)
}
