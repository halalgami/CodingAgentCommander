package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/rates"
	"github.com/halalgami/CodingAgentCommander/internal/stats"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// statsState holds Al Asturlab's engine, rebuilt when the ledger store is
// swapped (opt-in/out) and handed new rates when settings change.
type statsState struct {
	mu    sync.Mutex
	eng   *stats.Engine
	st    *store.Store
	book  *rates.Book
	ready bool
}

var errNoHistory = errors.New("history is off")

func (a *App) asturlabPath() string {
	return filepath.Join(filepath.Dir(configPath()), "asturlab.json")
}

func (a *App) statsBook() *rates.Book {
	a.stats.mu.Lock()
	defer a.stats.mu.Unlock()
	if !a.stats.ready {
		a.stats.book = rates.NewBook(rates.LoadSettings(a.asturlabPath()))
		a.stats.ready = true
	}
	return a.stats.book
}

// engine returns nil when history is off.
func (a *App) engine() *stats.Engine {
	st := a.ledger.currentStore()
	if st == nil {
		return nil
	}
	book := a.statsBook()
	a.stats.mu.Lock()
	defer a.stats.mu.Unlock()
	if a.stats.eng == nil || a.stats.st != st {
		a.stats.eng = stats.New(st, book, a.ledger.providers(), time.Now)
		a.stats.st = st
	}
	return a.stats.eng
}

type StatsOverviewView struct {
	Locked   bool           `json:"locked"`
	Overview stats.Overview `json:"overview"`
}

// Stats answers one Al Asturlab query (spec §4.3).
func (a *App) Stats(q stats.Query) (stats.Result, error) {
	e := a.engine()
	if e == nil {
		return stats.Result{Rows: []stats.Row{}}, errNoHistory
	}
	res, err := e.Run(q)
	if res.Rows == nil {
		res.Rows = []stats.Row{}
	}
	return res, err
}

func (a *App) StatsOverview(from, to string) StatsOverviewView {
	e := a.engine()
	if e == nil {
		return StatsOverviewView{Locked: true}
	}
	return StatsOverviewView{Overview: e.Overview(from, to)}
}

func (a *App) StatsModels(from, to, project string) []stats.ModelRow {
	e := a.engine()
	if e == nil {
		return []stats.ModelRow{}
	}
	return e.Models(from, to, project)
}

func (a *App) AsturlabSettings() rates.Settings { return a.statsBook().Settings() }

func (a *App) SetAsturlabSettings(s rates.Settings) error {
	if err := rates.SaveSettings(a.asturlabPath(), s); err != nil {
		return err
	}
	b := rates.NewBook(s)
	a.stats.mu.Lock()
	a.stats.book, a.stats.ready = b, true
	eng := a.stats.eng
	a.stats.mu.Unlock()
	if eng != nil {
		eng.SetBook(b)
	}
	return nil
}

// ExportStats writes a chart's rows (kind "chart", from q) or the full
// day × project × model table (kind "days") as csv or json to a file the
// user picks. "" when cancelled.
func (a *App) ExportStats(kind, format, from, to string, q stats.Query) (string, error) {
	e := a.engine()
	if e == nil {
		return "", errNoHistory
	}
	var days []stats.DayRow
	var rows []stats.Row
	if kind == "days" {
		days = e.Days(from, to)
	} else {
		res, err := e.Run(q)
		if err != nil {
			return "", err
		}
		rows = res.Rows
	}
	body, err := encodeExport(kind, format, days, rows)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("asturlab-%s-%s.%s", kind, time.Now().Format("2006-01-02"), format)
	dest, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{Title: "Export Al Asturlab data", DefaultFilename: name})
	if err != nil || dest == "" {
		return "", err
	}
	if strings.ToLower(filepath.Ext(dest)) != "."+format {
		dest += "." + format
	}
	return dest, os.WriteFile(dest, body, 0o600)
}

func encodeExport(kind, format string, days []stats.DayRow, rows []stats.Row) ([]byte, error) {
	switch format {
	case "json":
		var v any = rows
		if kind == "days" {
			v = days
		}
		return json.MarshalIndent(v, "", "  ")
	case "csv":
	default:
		return nil, fmt.Errorf("format %q not csv|json", format)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	i64 := func(n int64) string { return strconv.FormatInt(n, 10) }
	if kind == "days" {
		w.Write([]string{"day", "project", "provider", "model", "in", "out", "cacheRead", "cacheWrite", "costUSD", "priced", "activeMs", "turns", "commits"})
		for _, r := range days {
			cost := "" // unpriced: blank, never 0
			if r.Priced {
				cost = strconv.FormatFloat(r.CostUSD, 'f', 6, 64)
			}
			w.Write([]string{r.Day, r.Project, r.Provider, r.Model, i64(r.Tokens.In), i64(r.Tokens.Out), i64(r.Tokens.CacheRead),
				i64(r.Tokens.CacheWrite), cost, strconv.FormatBool(r.Priced), i64(r.ActiveMs), i64(r.Turns), i64(r.Commits)})
		}
	} else {
		w.Write([]string{"key", "label", "split", "value", "unpricedTokens"})
		for _, r := range rows {
			w.Write([]string{r.Key, r.Label, r.Split, strconv.FormatFloat(r.Value, 'f', -1, 64), i64(r.Unpriced)})
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
