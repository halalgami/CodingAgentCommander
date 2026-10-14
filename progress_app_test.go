package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

func countEvents(fe *fakeEmitter, name string) int {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	n := 0
	for _, e := range fe.events {
		if e == name {
			n++
		}
	}
	return n
}

func attachTestStore(t *testing.T, a *App) *store.Store {
	t.Helper()
	st, err := store.Create(a.ledgerDir(), store.Consent{StoreTitles: true}, nowFn)
	if err != nil {
		t.Fatal(err)
	}
	a.ledger.mu.Lock()
	a.ledger.store = st
	a.ledger.mu.Unlock()
	a.ledger.ing.SetStore(st)
	t.Cleanup(func() { st.Close() })
	return st
}

var nowFn = func() time.Time { return time.Now() }

func applyCommit(st *store.Store, root, sid string) string {
	id := repoid.ID{Key: repoid.KeyOf("path:" + root), Identity: "path:" + root, Root: root, Label: "p"}
	st.Apply(id, "/t/"+sid+".jsonl", store.Cursor{SessionID: sid}, false, []parse.Event{
		{Kind: parse.ToolUse, At: 1, ToolID: "c", ToolName: "Bash", Command: "git commit -m x"},
		{Kind: parse.TurnDuration, At: 2, UUID: sid + "t", Ms: 120 * 60000},
	}, 0)
	return id.Key
}

func TestBackfillUnlocksAreSilent(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	a.ledger.backfilling.Store(true)
	a.ledger.evaluate(key) // during backfill: deferred, no work
	if us := st.Unlocks(key); len(us) != 0 {
		t.Fatalf("evaluate ran during backfill: %v", us)
	}
	a.ledger.backfilling.Store(false)
	a.ledger.finishBackfill() // one silent pass over everything
	if n := countEvents(fe, "dash:achievement"); n != 0 {
		t.Fatalf("backfill emitted %d achievement events", n)
	}
	if len(st.Unlocks(key)) == 0 {
		t.Fatal("end-of-backfill pass did not record the unlock")
	}
	key2 := applyCommit(st, "/p2", "s2")
	a.ledger.evaluate(key2)
	a.ledger.evaluate(key2)
	if n := countEvents(fe, "dash:achievement"); n != 1 {
		t.Fatalf("live evaluation emitted %d, want exactly 1 (first_blood once)", n)
	}
}

func TestHeroEventOnlyOnLevelChange(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.Prompt, At: 3, SessionID: "s1"}}, false)
	a.ledger.evaluate(key)
	a.ledger.evaluate(key)
	if n := countEvents(fe, "dash:hero"); n != 1 {
		t.Fatalf("hero events %d, want 1 (first sight), then none until the level changes", n)
	}
	_ = live.Thinking
}

func TestHeroEventOnLootChange(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.Prompt, At: 3, SessionID: "s1"}}, false)
	a.ledger.evaluate(key)
	id := repoid.ID{Key: key, Identity: "path:/p1", Root: "/p1", Label: "p"}
	st.Apply(id, "/t/s1.jsonl", store.Cursor{SessionID: "s1"}, false, []parse.Event{
		{Kind: parse.ToolUse, At: 4, ToolID: "c2", ToolName: "Bash", Command: "git commit -m y"},
	}, 0)
	a.ledger.evaluate(key)
	a.ledger.evaluate(key)
	if n := countEvents(fe, "dash:hero"); n != 2 {
		t.Fatalf("hero events %d, want 2 (first sight, then the second commit), none on an unchanged re-evaluate", n)
	}
}

func TestForgetRecomputesCommander(t *testing.T) {
	a := ledgerTestApp(t)
	st := attachTestStore(t, a)
	k1 := applyCommit(st, "/p1", "s1")
	applyCommit(st, "/p2", "s2")
	before := a.Progress().Commander.XP
	st.RecordAccountUnlocks([]store.Unlock{{ID: "commander_10"}})
	if err := a.ForgetProject(k1); err != nil {
		t.Fatal(err)
	}
	after := a.Progress()
	if !(after.Commander.XP < before) {
		t.Fatalf("commander XP %v → %v; forgetting a project must lower it", before, after.Commander.XP)
	}
	if len(after.Commander.Unlocks) != 1 {
		t.Fatal("account unlock removed by forget")
	}
}

func TestProgressLockedWithoutHistory(t *testing.T) {
	a := ledgerTestApp(t)
	if !a.Progress().Locked {
		t.Fatal("progress not locked without history")
	}
}

// Minors round 2 (phase 3 review): no test drove a real level crossing through
// evaluate; a large stretch of work must re-emit the hero once.
func TestHeroEventOnLevelCrossing(t *testing.T) {
	a := ledgerTestApp(t)
	fe := a.emitter.(*fakeEmitter)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.Prompt, At: 3, SessionID: "s1"}}, false)
	a.ledger.evaluate(key)
	id := repoid.ID{Key: key, Identity: "path:/p1", Root: "/p1", Label: "p"}
	var evs []parse.Event
	for i := 0; i < 400; i++ {
		evs = append(evs, parse.Event{Kind: parse.TurnDuration, At: int64(10 + i), Ms: 60_000, UUID: fmt.Sprint("t", i)})
	}
	st.Apply(id, "/t/s1.jsonl", store.Cursor{SessionID: "s1"}, false, evs, 0)
	a.ledger.evaluate(key)
	if n := countEvents(fe, "dash:hero"); n != 2 {
		t.Fatalf("hero events %d, want 2 (first sight, then the level crossing)", n)
	}
}
