package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/quota"
)

func quotaApp(t *testing.T, util float64) *App {
	a := ledgerTestApp(t)
	a.quota = a.newQuotaSampler(map[string]quota.Probe{"anthropic": func(context.Context) ([]quota.Window, error) {
		return []quota.Window{{Key: "five_hour", Label: "Session", Util: util, ResetsAt: time.Now().Add(3 * time.Hour).Format(time.RFC3339)}}, nil
	}})
	return a
}

func TestQuotaNowListsAnthropicAndUnsupported(t *testing.T) {
	a := quotaApp(t, 42)
	a.quota.Tick(context.Background())
	v := a.QuotaNow()
	var anth, oll *ProviderQuota
	for i := range v.Providers {
		switch v.Providers[i].Provider {
		case "anthropic":
			anth = &v.Providers[i]
		case "ollama-cloud":
			oll = &v.Providers[i]
		}
	}
	if anth == nil || !anth.Supported || len(anth.Windows) != 1 || anth.Windows[0].Util != 42 || anth.At == "" {
		t.Fatalf("anthropic %+v", anth)
	}
	if oll == nil || oll.Supported {
		t.Fatalf("ollama %+v", oll)
	}
}

func TestQuotaHistorySeriesAndForecast(t *testing.T) {
	a := quotaApp(t, 0)
	attachTestStore(t, a)
	resets := time.Now().Add(5 * 24 * time.Hour).Format(time.RFC3339)
	st := quota.Store{Dir: a.quotaDir()}
	for i := 0; i <= 12; i++ {
		_ = st.Append("anthropic", quota.Sample{T: time.Now().Add(-2*time.Hour + time.Duration(i)*10*time.Minute),
			Windows: []quota.Window{{Key: "seven_day", Label: "Week", Weekly: true, Util: 10 + float64(i), ResetsAt: resets}}})
	}
	v := a.QuotaHistory("anthropic", 24)
	if v.Locked || len(v.Windows) != 1 {
		t.Fatalf("%+v", v)
	}
	w := v.Windows[0]
	if w.Key != "seven_day" || len(w.Points) != 13 || w.Points[0].U != 10 || w.Forecast == "" {
		t.Fatalf("%+v", w)
	}
}

func TestQuotaHistoryLockedWhenHistoryOff(t *testing.T) {
	a := quotaApp(t, 7)
	a.quota.Tick(context.Background())
	v := a.QuotaHistory("anthropic", 24)
	if !v.Locked || len(v.Windows) != 1 || len(v.Windows[0].Points) != 1 {
		t.Fatalf("%+v", v)
	}
}

func TestDeleteLedgerRemovesQuotaHistory(t *testing.T) {
	a := quotaApp(t, 9)
	attachTestStore(t, a)
	a.quota.Tick(context.Background())
	if _, err := os.Stat(a.quotaDir()); err != nil {
		t.Fatalf("no history written: %v", err)
	}
	if err := a.DeleteLedger(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.quotaDir())); !os.IsNotExist(err) {
		t.Fatalf("quota history survived delete: %v", err)
	}
}

func TestQuotaRefreshProbesOnDemandOnly(t *testing.T) {
	a := ledgerTestApp(t)
	calls := 0
	a.quota = a.newQuotaSampler(map[string]quota.Probe{"anthropic": func(context.Context) ([]quota.Window, error) {
		calls++
		return []quota.Window{{Key: "five_hour", Util: 12}}, nil
	}})
	if v := a.QuotaNow(); calls != 0 || len(v.Providers[0].Windows) != 0 {
		t.Fatalf("QuotaNow must not probe: calls %d", calls)
	}
	v := a.QuotaRefresh()
	if calls != 1 || len(v.Providers[0].Windows) != 1 {
		t.Fatalf("refresh: calls %d %+v", calls, v.Providers[0])
	}
	a.QuotaRefresh()
	if calls != 1 {
		t.Fatalf("second refresh within a minute probed again: %d", calls)
	}
}

func TestStartupDoesNotProbe(t *testing.T) {
	a := ledgerTestApp(t)
	called := false
	a.quota = a.newQuotaSampler(map[string]quota.Probe{"anthropic": func(context.Context) ([]quota.Window, error) {
		called = true
		return nil, nil
	}})
	a.startQuota(context.Background())
	time.Sleep(50 * time.Millisecond)
	if called {
		t.Fatal("startup probed the quota endpoint")
	}
}

func TestQuotaSampleDuringDeleteLeavesNothing(t *testing.T) {
	// Review #7: a check landing while history is deleted must not recreate it.
	a := quotaApp(t, 9)
	attachTestStore(t, a)
	a.ledger.consentMu.Lock()
	done := make(chan struct{})
	go func() { a.quota.Tick(context.Background()); close(done) }()
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(a.quotaDir()); !os.IsNotExist(err) {
		t.Fatalf("sample written while a history change held the lock: %v", err)
	}
	a.ledger.stop()
	if err := store.DeleteAll(a.ledgerDir()); err != nil {
		t.Fatal(err)
	}
	a.ledger.consentMu.Unlock()
	<-done
	if _, err := os.Stat(a.quotaDir()); !os.IsNotExist(err) {
		t.Fatalf("quota history recreated after delete: %v", err)
	}
}
