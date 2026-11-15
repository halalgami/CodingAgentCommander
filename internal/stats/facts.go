package stats

import "github.com/halalgami/CodingAgentCommander/internal/ledger/store"

// fact is one model's slice of one session-day. Counters (turns, commits, …)
// are attributed by the model's token share of that day (the ledger has no
// per-model turn counts); a day without a per-model split (pre-parser-v6) is
// one fact for model "unknown".
type fact struct {
	day, project, session, model, provider string
	tok                                    store.TokenCounts
	share                                  float64
	c                                      *store.Counters
	// major: the largest-share fact of its session-day. Whole-number
	// counters (commits, turns) go to it whole in the per-row export.
	major bool
}

func flatten(src Source, catalog func(string) string) ([]fact, map[string]string) {
	labels := map[string]string{}
	var out []fact
	for _, p := range src.Projects() {
		labels[p.Key] = p.Label
		for _, s := range src.Sessions(p.Key) {
			for d, c := range s.Days {
				c := c
				out = append(out, splitDay(d, p.Key, s.ID, &c, s.TokensByModel, catalog)...)
			}
		}
	}
	return out, labels
}

func splitDay(d, key, sid string, c *store.Counters, session map[string]store.TokenCounts, catalog func(string) string) []fact {
	out := splitTokens(d, key, sid, c, session, catalog)
	best := 0
	for i := range out {
		if out[i].share > out[best].share {
			best = i
		}
	}
	if len(out) > 0 {
		out[best].major = true
	}
	return out
}

// prorate splits a pre-v6 day's tokens by the session's whole-life model mix
// (sessions kept per-model totals before parser v6).
func prorate(t store.TokenCounts, session map[string]store.TokenCounts) map[string]store.TokenCounts {
	var all int64
	for _, x := range session {
		all += x.Total()
	}
	if all == 0 {
		return nil
	}
	out := map[string]store.TokenCounts{}
	var used store.TokenCounts
	var last string
	for m, x := range session {
		f := float64(x.Total()) / float64(all)
		p := store.TokenCounts{In: int64(float64(t.In) * f), Out: int64(float64(t.Out) * f), CacheRead: int64(float64(t.CacheRead) * f), CacheWrite: int64(float64(t.CacheWrite) * f)}
		out[m] = p
		used.In, used.Out, used.CacheRead, used.CacheWrite = used.In+p.In, used.Out+p.Out, used.CacheRead+p.CacheRead, used.CacheWrite+p.CacheWrite
		if last == "" || m < last {
			last = m
		}
	}
	// rounding remainder to one model so the day still sums exactly
	r := out[last]
	r.In, r.Out, r.CacheRead, r.CacheWrite = r.In+t.In-used.In, r.Out+t.Out-used.Out, r.CacheRead+t.CacheRead-used.CacheRead, r.CacheWrite+t.CacheWrite-used.CacheWrite
	out[last] = r
	return out
}

func splitTokens(d, key, sid string, c *store.Counters, session map[string]store.TokenCounts, catalog func(string) string) []fact {
	mk := func(model string, t store.TokenCounts, share float64) fact {
		return fact{day: d, project: key, session: sid, model: model, provider: ProviderOf(model, catalog), tok: t, share: share, c: c}
	}
	total := c.Tokens.Total()
	by := c.TokensByModel
	if len(by) == 0 && total > 0 {
		by = prorate(c.Tokens, session)
	}
	if len(by) == 0 {
		return []fact{mk("unknown", c.Tokens, 1)}
	}
	var out []fact
	var sum store.TokenCounts
	for m, t := range by {
		sum.In += t.In
		sum.Out += t.Out
		sum.CacheRead += t.CacheRead
		sum.CacheWrite += t.CacheWrite
		share := 1 / float64(len(by))
		if total > 0 {
			share = float64(t.Total()) / float64(total)
		}
		out = append(out, mk(m, t, share))
	}
	// Tokens no model claimed (e.g. <synthetic>) stay counted, as "unknown".
	left := store.TokenCounts{In: c.Tokens.In - sum.In, Out: c.Tokens.Out - sum.Out,
		CacheRead: c.Tokens.CacheRead - sum.CacheRead, CacheWrite: c.Tokens.CacheWrite - sum.CacheWrite}
	if left.Total() > 0 && total > 0 {
		out = append(out, mk("unknown", left, float64(left.Total())/float64(total)))
	}
	return out
}
