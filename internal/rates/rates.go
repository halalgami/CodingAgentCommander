// Package rates is the API-equivalent price table for Al Asturlab. Every
// figure is an estimate as true as PricedAt; past StaleAfter the UI flags
// every dollar as possibly stale. A model with no rate is never $0: callers
// show its tokens only.
package rates

import (
	"sort"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
)

// Rate is USD per million tokens.
type Rate struct {
	In         float64 `json:"in"`
	Out        float64 `json:"out"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// Cost is the API-equivalent price of t at this rate.
func (r Rate) Cost(t store.TokenCounts) float64 {
	return (float64(t.In)*r.In + float64(t.Out)*r.Out + float64(t.CacheRead)*r.CacheRead + float64(t.CacheWrite)*r.CacheWrite) / 1e6
}

// pricedAt is the date of the source the table was copied from (Anthropic's
// first-party list prices as published 2026-10-06), not the day it was typed.
var pricedAt = time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)

const StaleAfter = 60 * 24 * time.Hour

func PricedAt() time.Time      { return pricedAt }
func Stale(now time.Time) bool { return now.Sub(pricedAt) > StaleAfter }
func r(in, out, read float64) Rate {
	return Rate{In: in, Out: out, CacheRead: read, CacheWrite: in * 1.25}
}

// table: Anthropic first-party rates. Cache writes are the 5-minute rate
// (1.25× input). Cache reads are the published figure where one is listed,
// else 0.1× input. Ollama Cloud and Zen ship no rates: user overrides only.
var table = map[string]Rate{
	"claude-fable-5-1":  r(10, 50, 0.25),
	"claude-fable-5":    r(10, 50, 1.0),
	"claude-opus-5-5":   r(4, 20, 0.20),
	"claude-opus-5":     r(5, 25, 0.5),
	"claude-opus-4-8":   r(5, 25, 0.5),
	"claude-opus-4-7":   r(5, 25, 0.5),
	"claude-opus-4-6":   r(5, 25, 0.5),
	"claude-sonnet-5-5": r(2, 10, 0.20),
	"claude-sonnet-5":   r(2, 10, 0.20),
	"claude-sonnet-4-6": r(3, 15, 0.3),
	"claude-haiku-5-5":  r(0.10, 0.50, 0.01),
	"claude-haiku-4-5":  r(1, 5, 0.1),
}

// keys longest first so "claude-opus-5-5" wins over "claude-opus-5".
var keys = func() []string {
	ks := make([]string, 0, len(table))
	for k := range table {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return len(ks[i]) > len(ks[j]) })
	return ks
}()

type Book struct{ s Settings }

func NewBook(s Settings) *Book { return &Book{s: s} }

// Settings returns the settings the book was built from.
func (b *Book) Settings() Settings { return b.s }

// Rate resolves: exact-model override → provider override → shipped table
// (exact id, then the longest id the model starts with, after stripping a
// Bedrock "us."/"eu."/"anthropic." prefix).
func (b *Book) Rate(model, provider string) (Rate, bool) {
	if x, ok := b.s.Overrides[model]; ok {
		return x, true
	}
	if x, ok := b.s.Overrides["provider:"+provider]; ok {
		return x, true
	}
	m := model
	if i := strings.Index(m, "anthropic."); i >= 0 {
		m = m[i+len("anthropic."):]
	}
	if x, ok := table[m]; ok {
		return x, true
	}
	for _, k := range keys {
		if strings.HasPrefix(m, k+"-") || strings.HasPrefix(m, k+"@") || strings.HasPrefix(m, k+":") {
			return table[k], true
		}
	}
	return Rate{}, false
}
