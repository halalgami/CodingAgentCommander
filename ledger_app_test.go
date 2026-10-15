package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
	"github.com/halalgami/CodingAgentCommander/internal/winstate"
)

func TestLedgerOptInLifecycleAndStatsFields(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(cfgDir, "config.toml"))
	a := NewApp()
	a.emitter = &fakeEmitter{}
	l := a.ledger
	l.projectsRoot = filepath.Join(t.TempDir(), "projects")

	if st := a.LedgerStatus(); st.Enabled {
		t.Fatal("enabled before opt-in")
	}
	if _, err := os.Stat(a.ledgerDir()); !os.IsNotExist(err) {
		t.Fatal("ledger dir exists before consent")
	}

	// A Commander session with a transcript the tracker can tail.
	cwd := t.TempDir()
	sid := "11111111-2222-4333-8444-555555555555"
	tp := filepath.Join(l.projectsRoot, "enc", sid+".jsonl")
	os.MkdirAll(filepath.Dir(tp), 0o755)
	os.WriteFile(tp, []byte(`{"type":"ai-title","aiTitle":"Fix <b>VAT</b>","sessionId":"`+sid+`"}`+"\n"+
		`{"type":"assistant","timestamp":"2026-10-09T09:00:00Z","sessionId":"`+sid+`","cwd":"`+strings.ReplaceAll(cwd, `\`, `\\`)+`","message":{"model":"m","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`+"\n"), 0o600)
	a.sessions["@1"] = &sessionRec{Name: "p", Cwd: cwd, Status: "active", ClaudeSessionID: sid, LaunchedAt: time.Now()}
	l.ing.Watch(tp)
	l.ing.Tail(t.Context())

	st := a.SessionStats("@1")
	if st.SessionID != sid || st.Title != "Fix <b>VAT</b>" || st.Activity != "tool" || st.Tool != "Bash" || !strings.HasPrefix(st.Target, "go test") {
		t.Fatalf("stats = %+v", st)
	}

	a.handleNotify([]byte(`{"session_id":"` + sid + `","hook_event_name":"Notification","notification_type":"permission_prompt"}`))
	if a.SessionStats("@1").Activity != "permission" {
		t.Fatal("permission notification not applied")
	}

	status, err := a.SetLedgerConsent(true, false)
	if err != nil || !status.Enabled {
		t.Fatalf("opt-in: %+v %v", status, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !a.LedgerStatus().Progress.Done && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !a.LedgerStatus().Progress.Done || len(a.DashSnapshot().Projects) != 1 {
		t.Fatalf("backfill: %+v", a.LedgerStatus())
	}

	if err := a.DeleteLedger(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.ledgerDir()); !os.IsNotExist(err) {
		t.Fatal("delete left the ledger dir")
	}
	if a.LedgerStatus().Enabled {
		t.Fatal("still enabled after delete")
	}
}

func ledgerTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	a := NewApp()
	a.emitter = &fakeEmitter{}
	a.ledger.projectsRoot = filepath.Join(t.TempDir(), "projects")
	a.ledger.ing.Root = a.ledger.projectsRoot
	return a
}

func waitBackfill(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !a.LedgerStatus().Progress.Done && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOptOutPersistsAcrossRestart(t *testing.T) {
	a := ledgerTestApp(t)
	if _, err := a.SetLedgerConsent(true, true); err != nil {
		t.Fatal(err)
	}
	waitBackfill(t, a)
	if _, err := a.SetLedgerConsent(false, true); err != nil {
		t.Fatal(err)
	}
	if store.Enabled(a.ledgerDir()) {
		t.Fatal("opt-out not persisted: the next launch would resume recording")
	}
	if st := a.LedgerStatus(); st.Enabled || !st.HasData {
		t.Fatalf("status after opt-out = %+v; want disabled with data still deletable", st)
	}
	if _, err := a.SetLedgerConsent(true, true); err != nil || !store.Enabled(a.ledgerDir()) {
		t.Fatalf("re-opt-in failed: %v", err)
	}
}

func TestConcurrentOptInAttachesWritableStore(t *testing.T) {
	a := ledgerTestApp(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = a.SetLedgerConsent(true, true) }()
	}
	wg.Wait()
	waitBackfill(t, a)
	if a.LedgerStatus().ReadOnly {
		t.Fatal("concurrent opt-ins left the attached store read-only")
	}
	a.ledger.stop()
}

func TestStopAfterClearAdoptsNewSessionID(t *testing.T) {
	a := ledgerTestApp(t)
	a.notifier = &fakeNotifier{}
	a.sessions["@1"] = &sessionRec{Name: "p", Cwd: "/tmp/one", Status: "active", ClaudeSessionID: "before-clear"}
	a.handleNotify([]byte(`{"session_id":"after-clear","cwd":"/tmp/one","hook_event_name":"Stop"}`))
	r := a.sessions["@1"]
	if r.Status != "finished" || r.ClaudeSessionID != "after-clear" {
		t.Fatalf("after /clear: status=%s sid=%s; want finished + adopted id", r.Status, r.ClaudeSessionID)
	}
	if got, _ := a.windows.Get("@1"); got.SessionID != "after-clear" {
		t.Fatalf("adopted id not persisted: %+v", got)
	}
}

func TestSwapWithoutTranscriptLaunchesFresh(t *testing.T) {
	keyring.MockInit()
	a := ledgerTestApp(t)
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	a.sessions["@1"] = &sessionRec{Name: "p", Cwd: "/tmp/never-prompted", Model: "claude-opus-4-8", Provider: "anthropic", ClaudeSessionID: "launched-but-silent", Status: "active"}
	if _, err := a.SwapModel("@1", "claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	cmd := ch.launched[0].Command
	for _, arg := range cmd {
		if arg == "--resume" {
			t.Fatalf("resumed a conversation that has no transcript: %v", cmd)
		}
	}
}

func TestSessionIDSurvivesRestartViaWinstate(t *testing.T) {
	keyring.MockInit()
	a := ledgerTestApp(t)
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	a.host = &captureHost{}
	m, _ := a.cfg.Model("claude-opus-4-8")
	info, err := a.startSession("/tmp/p", m, nil)
	if err != nil {
		t.Fatal(err)
	}
	sid := a.sessions[info.WindowID].ClaudeSessionID
	if got, _ := a.windows.Get(info.WindowID); got.SessionID != sid {
		t.Fatalf("session id not recorded in winstate: %+v", got)
	}
	// A fresh process adopts the surviving window.
	b := ledgerTestApp(t)
	b.loadConfigFrom("example.config.toml")
	b.windows.Set("@7", winstate.Record{Model: "claude-opus-4-8", SessionID: "kept"})
	b.reconcile([]tmux.WindowState{{ID: "@7", Name: "p", Cwd: "/tmp/p"}})
	if b.sessions["@7"] == nil || b.sessions["@7"].ClaudeSessionID != "kept" {
		t.Fatalf("reconcile lost the session id: %+v", b.sessions["@7"])
	}
}

// Minors round 2 (phase 3 review): the per-session hero signatures were never
// pruned, so every session ever seen stayed in memory.
func TestHeroSignaturesPrunedWhenSessionsGo(t *testing.T) {
	a := NewApp()
	l := a.ledger
	l.heroes["gone"] = heroSig{level: 3}
	l.heroes["kept"] = heroSig{level: 4}
	l.queueGone([]string{"gone"})
	l.heroMu.Lock()
	_, g := l.heroes["gone"]
	_, k := l.heroes["kept"]
	l.heroMu.Unlock()
	if g || !k {
		t.Fatalf("gone present=%v kept present=%v", g, k)
	}
}

// Minors round 2 (phase 3 review): opting out and back in during a backfill let
// the OLD backfill's end clear the flag the NEW one had set, so the new backfill
// fired achievement toasts for the whole history.
func TestStaleBackfillCannotEndTheCurrentOne(t *testing.T) {
	a := NewApp()
	l := a.ledger
	old := l.beginBackfill()
	cur := l.beginBackfill() // re-attached before the old one finished
	l.endBackfill(old)
	if !l.backfilling.Load() {
		t.Fatal("a stale backfill ended the current one")
	}
	l.endBackfill(cur)
	if l.backfilling.Load() {
		t.Fatal("the current backfill did not end")
	}
}
