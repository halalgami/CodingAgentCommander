package quota

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func fakeProbe(util *float64, fail *bool, calls *int32) Probe {
	return func(ctx context.Context) ([]Window, error) {
		atomic.AddInt32(calls, 1)
		if fail != nil && *fail {
			return nil, errors.New("offline")
		}
		return []Window{{Key: "five_hour", Label: "Session", Util: *util}}, nil
	}
}

func TestTickPersistsWhenAllowed(t *testing.T) {
	u, n, seen := 30.0, int32(0), 0
	s := &Sampler{Probes: map[string]Probe{"anthropic": fakeProbe(&u, nil, &n)}, Store: Store{Dir: t.TempDir()},
		Persist: func() bool { return true }, OnSample: func(string, Sample) { seen++ }}
	s.Tick(context.Background())
	got, _ := s.Store.Read("anthropic", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if len(got) != 1 || seen != 1 {
		t.Fatalf("persisted %d, OnSample %d", len(got), seen)
	}
}

func TestTickNoPersistWhenOff(t *testing.T) {
	u, n := 30.0, int32(0)
	s := &Sampler{Probes: map[string]Probe{"anthropic": fakeProbe(&u, nil, &n)}, Store: Store{Dir: t.TempDir()},
		Persist: func() bool { return false }}
	s.Tick(context.Background())
	got, _ := s.Store.Read("anthropic", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if len(got) != 0 {
		t.Fatal("wrote history while off")
	}
	if x, _, ok := s.Latest("anthropic"); !ok || x.Windows[0].Util != 30 {
		t.Fatalf("latest %+v %v", x, ok)
	}
}

func TestTickProbeErrorKeepsPrevious(t *testing.T) {
	u, fail, n := 30.0, false, int32(0)
	s := &Sampler{Probes: map[string]Probe{"anthropic": fakeProbe(&u, &fail, &n)}, Store: Store{Dir: t.TempDir()},
		Persist: func() bool { return true }}
	s.Tick(context.Background())
	fail = true
	s.Tick(context.Background())
	x, errText, ok := s.Latest("anthropic")
	if !ok || x.Windows[0].Util != 30 || errText != "offline" {
		t.Fatalf("latest %+v %q %v", x, errText, ok)
	}
	got, _ := s.Store.Read("anthropic", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if len(got) != 1 {
		t.Fatalf("appended %d", len(got))
	}
}

func TestRefreshProbesOnlyWhenStale(t *testing.T) {
	u, n := 1.0, int32(0)
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	s := &Sampler{Probes: map[string]Probe{"anthropic": fakeProbe(&u, nil, &n)}, Store: Store{Dir: t.TempDir()},
		Persist: func() bool { return false }, Now: func() time.Time { return clock }}
	s.Refresh(context.Background(), time.Minute)
	s.Refresh(context.Background(), time.Minute) // fresh: no second call
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("calls %d, want 1", n)
	}
	clock = clock.Add(61 * time.Second)
	s.Refresh(context.Background(), time.Minute)
	if atomic.LoadInt32(&n) != 2 {
		t.Fatalf("calls %d, want 2 after the minute", n)
	}
}

func TestRefreshAfterErrorWaitsTooAsks(t *testing.T) {
	// A failed probe also counts as a check: no hammering a rejecting endpoint.
	u, fail, n := 1.0, true, int32(0)
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	s := &Sampler{Probes: map[string]Probe{"anthropic": fakeProbe(&u, &fail, &n)}, Store: Store{Dir: t.TempDir()},
		Persist: func() bool { return false }, Now: func() time.Time { return clock }}
	s.Refresh(context.Background(), time.Minute)
	s.Refresh(context.Background(), time.Minute)
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("calls %d, want 1", n)
	}
}
