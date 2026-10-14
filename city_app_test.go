package main

import (
	"testing"
)

func TestCityViewLockedWithoutHistory(t *testing.T) {
	a := ledgerTestApp(t)
	v := a.CityView()
	if !v.Locked || len(v.Districts) != 0 {
		t.Fatalf("history off: %+v", v)
	}
}

func TestCityViewBuildsDistrictsAndDashCityIsThrottled(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	v := a.CityView()
	if v.Locked || len(v.Districts) != 1 || v.Districts[0].Key != key || v.Districts[0].Plot != 1 || len(v.Districts[0].Buildings) != 1 {
		t.Fatalf("city %+v", v)
	}
	a.ledger.queueProject(key)
	for n := 1; n <= 7; n++ {
		a.ledger.flushPending(n)
	}
	if countEvents(fe, "dash:city") != 0 {
		t.Fatal("dash:city before the 2 s step")
	}
	a.ledger.flushPending(8)
	a.ledger.flushPending(16)
	if n := countEvents(fe, "dash:city"); n != 1 {
		t.Fatalf("dash:city %d, want 1 (only steps with changes)", n)
	}
}

func TestLedgerChangesRefreshTheCity(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	steps := []struct {
		name string
		do   func() error
	}{
		{"forget", func() error { return a.ForgetProject(key) }},
		{"exclude", func() error { return a.SetLedgerExcluded([]string{"/p2"}) }},
		{"consent off", func() error { _, err := a.SetLedgerConsent(false, false); return err }},
		{"delete", a.DeleteLedger},
	}
	for _, s := range steps {
		before := countEvents(fe, "dash:city")
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if countEvents(fe, "dash:city") != before+1 {
			t.Fatalf("%s did not emit dash:city", s.name)
		}
	}
}

func TestProvidersResolvesTheCatalogOnce(t *testing.T) {
	a := ledgerTestApp(t)
	ms := a.snapshotModels()
	if len(ms) == 0 {
		t.Skip("empty catalog")
	}
	p := a.ledger.providers()
	if got := p(ms[0].ID); got != ms[0].Provider {
		t.Fatalf("provider of %s = %q, want %q", ms[0].ID, got, ms[0].Provider)
	}
	if p("no-such-model") != "" {
		t.Fatal("unknown model should have no provider")
	}
}
