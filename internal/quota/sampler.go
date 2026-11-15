package quota

import (
	"context"
	"sync"
	"time"
)

// Probe asks one provider's official quota endpoint for its windows.
type Probe func(ctx context.Context) ([]Window, error)

// Sampler probes providers ON DEMAND only (owner, 2026-11-15: no background
// polling of the quota endpoint — only when a view that shows quota opens).
// It keeps the latest sample in memory and, when Persist says so, appends it
// to Store. A failed probe keeps the previous sample and records the error.
type Sampler struct {
	Probes   map[string]Probe
	Store    Store
	Persist  func() bool
	Now      func() time.Time
	OnSample func(provider string, s Sample)
	// Save, when set, replaces Persist+Store.Append so the caller can hold its
	// own lock across the "may I write?" check and the write.
	Save func(provider string, s Sample)

	mu        sync.Mutex
	refreshMu sync.Mutex // one refresh at a time: concurrent opens share it
	latest    map[string]Sample
	errs      map[string]string
	checked   map[string]time.Time // last probe attempt, success or not
	pruned    time.Time
}

func (s *Sampler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Tick probes every provider once.
func (s *Sampler) Tick(ctx context.Context) {
	for name, p := range s.Probes {
		s.tickOne(ctx, name, p)
	}
}

func (s *Sampler) tickOne(ctx context.Context, name string, p Probe) {
	ws, err := p(ctx)
	s.mu.Lock()
	if s.latest == nil {
		s.latest, s.errs, s.checked = map[string]Sample{}, map[string]string{}, map[string]time.Time{}
	}
	s.checked[name] = s.now()
	if err != nil {
		s.errs[name] = err.Error()
		s.mu.Unlock()
		return
	}
	x := Sample{T: s.now(), Windows: ws}
	s.latest[name], s.errs[name] = x, ""
	s.mu.Unlock()
	if s.Save != nil {
		s.Save(name, x)
	} else if s.Persist != nil && s.Persist() {
		_ = s.Store.Append(name, x)
	}
	if s.OnSample != nil {
		s.OnSample(name, x)
	}
}

// Latest returns the newest good sample, the last probe error ("" if the last
// probe succeeded) and whether any sample exists.
func (s *Sampler) Latest(provider string) (Sample, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.latest[provider]
	return x, s.errs[provider], ok
}

// Refresh probes each provider whose last check (success or failure) is older
// than maxAge, so opening several quota views in a row asks once. History
// older than Retention is pruned at most once a day, here.
func (s *Sampler) Refresh(ctx context.Context, maxAge time.Duration) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	now := s.now()
	for name, p := range s.Probes {
		s.mu.Lock()
		last, ok := s.checked[name]
		s.mu.Unlock()
		if ok && now.Sub(last) < maxAge {
			continue
		}
		s.tickOne(ctx, name, p)
	}
	if now.Sub(s.pruned) >= 24*time.Hour {
		_ = s.Store.Prune(now)
		s.pruned = now
	}
}
