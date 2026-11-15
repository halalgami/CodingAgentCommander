package main

import (
	"context"
	"path/filepath"
	"sort"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/quota"
)

// Providers in the quota card. Only Anthropic has an official quota endpoint
// (/api/oauth/usage); the others are shown as "No official quota", never guessed.
var quotaProviders = []struct {
	id, label string
}{{"anthropic", "Anthropic"}, {"bedrock", "Bedrock"}, {"ollama-cloud", "Ollama Cloud"}, {"opencode-go", "OpenCode Go"}}

type ProviderQuota struct {
	Provider  string         `json:"provider"`
	Label     string         `json:"label"`
	Supported bool           `json:"supported"`
	Windows   []quota.Window `json:"windows"`
	At        string         `json:"at"`
	Error     string         `json:"error"`
}

type QuotaNowView struct {
	Providers []ProviderQuota `json:"providers"`
}

type QuotaPoint struct {
	T string  `json:"t"`
	U float64 `json:"u"`
}

type WindowSeries struct {
	Key      string       `json:"key"`
	Label    string       `json:"label"`
	Weekly   bool         `json:"weekly"`
	ResetsAt string       `json:"resetsAt"`
	Points   []QuotaPoint `json:"points"`
	Forecast string       `json:"forecast"` // RFC3339 or ""
	Note     string       `json:"note"`
}

type QuotaHistoryView struct {
	Windows []WindowSeries `json:"windows"`
	Locked  bool           `json:"locked"` // history off: points are the latest sample only
}

func (a *App) quotaDir() string { return filepath.Join(a.ledgerDir(), "quota") }

func (a *App) newQuotaSampler(probes map[string]quota.Probe) *quota.Sampler {
	return &quota.Sampler{
		Probes:  probes,
		Store:   quota.Store{Dir: a.quotaDir()},
		Persist: func() bool { return store.Enabled(a.ledgerDir()) },
		// Under consentMu, as opt-out and DeleteLedger are: a check that lands
		// while history is being deleted must not recreate it.
		Save: func(p string, x quota.Sample) {
			a.ledger.consentMu.Lock()
			defer a.ledger.consentMu.Unlock()
			if store.Enabled(a.ledgerDir()) {
				_ = quota.Store{Dir: a.quotaDir()}.Append(p, x)
			}
		},
		OnSample: func(p string, s quota.Sample) {
			max := 0.0
			for _, w := range s.Windows {
				if w.Util > max {
					max = w.Util
				}
			}
			if a.emitter != nil {
				a.emitter.Emit("quota:sample", map[string]any{"provider": p, "maxUtil": max})
			}
		},
	}
}

// anthropicProbe reuses the Usage drawer's fetch.
func (a *App) anthropicProbe(context.Context) ([]quota.Window, error) {
	u, err := a.PlanUsage()
	if err != nil {
		return nil, err
	}
	out := make([]quota.Window, 0, len(u.Windows))
	for _, w := range u.Windows {
		out = append(out, quota.Window{Key: w.Key, Label: w.Label, Weekly: w.Weekly, Util: w.Utilization, ResetsAt: w.ResetsAt})
	}
	return out, nil
}

// startQuota only builds the sampler. It never probes: the quota endpoint is
// asked only when a view that shows quota opens (QuotaRefresh) — owner,
// 2026-11-15, to avoid background polling of an account endpoint.
func (a *App) startQuota(context.Context) {
	if a.quota == nil {
		a.quota = a.newQuotaSampler(map[string]quota.Probe{"anthropic": a.anthropicProbe})
	}
}

// quotaMaxAge: opening several quota views in a row asks the endpoint once.
const quotaMaxAge = time.Minute

// QuotaRefresh asks each provider's quota endpoint if the last check is older
// than a minute, then returns QuotaNow. Called when a quota view opens.
func (a *App) QuotaRefresh() QuotaNowView {
	if a.quota != nil {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		a.quota.Refresh(ctx, quotaMaxAge)
	}
	return a.QuotaNow()
}

// QuotaNow returns the latest sample per provider. No network.
func (a *App) QuotaNow() QuotaNowView {
	var v QuotaNowView
	for _, p := range quotaProviders {
		pq := ProviderQuota{Provider: p.id, Label: p.label}
		if a.quota != nil {
			if _, ok := a.quota.Probes[p.id]; ok {
				pq.Supported = true
				x, errText, has := a.quota.Latest(p.id)
				pq.Error = errText
				if has {
					pq.Windows, pq.At = x.Windows, x.T.Format(time.RFC3339)
				}
			}
		}
		v.Providers = append(v.Providers, pq)
	}
	return v
}

// QuotaHistory returns each window's samples over the last hours (1..168)
// with a forecast. With history off it carries only the latest sample.
func (a *App) QuotaHistory(provider string, hours int) QuotaHistoryView {
	hours = min(max(hours, 1), 168)
	now := time.Now()
	var samples []quota.Sample
	v := QuotaHistoryView{Locked: !store.Enabled(a.ledgerDir())}
	if !v.Locked {
		samples, _ = quota.Store{Dir: a.quotaDir()}.Read(provider, now.Add(-time.Duration(hours)*time.Hour), now.Add(time.Minute))
	}
	if a.quota != nil {
		if x, _, ok := a.quota.Latest(provider); ok && (len(samples) == 0 || x.T.After(samples[len(samples)-1].T)) {
			samples = append(samples, x)
		}
	}
	byKey := map[string]*WindowSeries{}
	var keys []string
	for _, s := range samples {
		for _, w := range s.Windows {
			ws := byKey[w.Key]
			if ws == nil {
				ws = &WindowSeries{Key: w.Key}
				byKey[w.Key] = ws
				keys = append(keys, w.Key)
			}
			ws.Label, ws.Weekly, ws.ResetsAt = w.Label, w.Weekly, w.ResetsAt
			ws.Points = append(ws.Points, QuotaPoint{T: s.T.Format(time.RFC3339), U: w.Util})
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		ws := byKey[k]
		f := quota.ForecastOf(samples, k, now)
		if f.OK {
			ws.Forecast = f.HitAt.Format(time.RFC3339)
		}
		ws.Note = f.Note
		v.Windows = append(v.Windows, *ws)
	}
	return v
}
