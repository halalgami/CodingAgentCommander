package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCompanionSnapshotCounts(t *testing.T) {
	a := NewApp()
	a.setCompanionCountsForTest(2, 1, true, false) // running, finished, awaiting, error
	js := a.companionStateJSON()
	var got map[string]any
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatal(err)
	}
	if got["running"].(float64) != 2 || got["finished"].(float64) != 1 || got["awaiting"] != true {
		t.Fatalf("bad snapshot: %s", js)
	}
}

func TestCompanionConfigRoundTripAndClamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	// defaults when absent: kind off, and the legacy Enabled mirror false
	got := loadCompanionConfig()
	if got.Kind != KindOff || got.Enabled || got.Size != 340 || got.Opacity != 1.0 || got.Jiggle.Bust != 1.0 {
		t.Fatalf("defaults wrong: %+v", got)
	}
	// clamp out-of-range on save-load
	c := CompanionConfig{Enabled: false, Size: 9999, Opacity: 5, ModelPath: "/x.model"}
	c.Jiggle.Hair = 99
	if err := saveCompanionConfig(c.clamped()); err != nil {
		t.Fatal(err)
	}
	back := loadCompanionConfig()
	if back.Size != 520 || back.Opacity != 1.0 || back.Jiggle.Hair != 3 || back.Enabled {
		t.Fatalf("clamp/roundtrip wrong: %+v", back)
	}
}

func TestCompanionConfigCorruptFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	os.WriteFile(companionConfigPath(), []byte("{not json"), 0o644)
	if got := loadCompanionConfig(); got.Size != 340 {
		t.Fatalf("corrupt should fall back to defaults, got %+v", got)
	}
}

// TestCompanionKindMigrationAndClamp drives the on-disk config through
// loadCompanionConfig, which is where migration (§6.1) and clamping meet.
// companionHostOS is pinned per row so the darwin-only migration is provable
// on the Windows CI runner too.
func TestCompanionKindMigrationAndClamp(t *testing.T) {
	defer func(v bool) { avatarKindAvailable = v }(avatarKindAvailable)
	avatarKindAvailable = true

	realDir := t.TempDir() // a directory that exists, for the PackPath cases
	gone := filepath.Join(realDir, "vanished")

	// Paths go into the JSON fixtures through json.Marshal, never raw
	// concatenation: a Windows temp path is C:\Users\..., and \U and \A are
	// invalid JSON escapes, so the raw form made the fixture unparseable and the
	// loader fell back to defaults. The rows then failed on the CI runner only —
	// darwin paths contain no backslashes, so the bug was invisible locally.
	// Returns the path already quoted.
	jsonStr := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal fixture path %q: %v", s, err)
		}
		return string(b)
	}

	cases := []struct {
		name     string
		file     string // "" means: write no companion.json at all
		hostOS   string
		wantKind string
		wantPack string
		wantEnab bool
	}{
		{"new install, no file", "", "darwin", KindOff, "", false},
		{"legacy enabled on darwin", `{"enabled":true,"size":340}`, "darwin", KindAvatar, "", true},
		{"legacy enabled off darwin", `{"enabled":true,"size":340}`, "windows", KindOff, "", false},
		{"legacy disabled", `{"enabled":false}`, "darwin", KindOff, "", false},
		{"explicit kind wins over enabled", `{"kind":"panel","enabled":false}`, "darwin", KindPanel, "", false},
		{"unknown kind snaps off", `{"kind":"hologram"}`, "darwin", KindOff, "", false},
		{"vanished pack path cleared", `{"kind":"panel","packPath":` + jsonStr(gone) + `}`, "darwin", KindPanel, "", false},
		{"live pack path kept", `{"kind":"panel","packPath":` + jsonStr(realDir) + `}`, "darwin", KindPanel, realDir, false},
		{"avatar mirrors enabled", `{"kind":"avatar"}`, "darwin", KindAvatar, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
			old := companionHostOS
			companionHostOS = tc.hostOS
			defer func() { companionHostOS = old }()

			if tc.file != "" {
				if err := os.WriteFile(companionConfigPath(), []byte(tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := loadCompanionConfig()
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.PackPath != tc.wantPack {
				t.Errorf("PackPath = %q, want %q", got.PackPath, tc.wantPack)
			}
			if got.Enabled != tc.wantEnab {
				t.Errorf("Enabled mirror = %v, want %v", got.Enabled, tc.wantEnab)
			}
		})
	}
}

func TestCompanionFinishSeqAndName(t *testing.T) {
	a := NewApp()
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "alpha", Cwd: "/tmp/a", Status: "active"},
		"@2": {Name: "beta", Cwd: "/tmp/b", Status: "active"},
	}

	decode := func() map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(a.companionStateJSON()), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	// Before any finish: seq 0, no name.
	s0 := decode()
	if s0["finishSeq"].(float64) != 0 || s0["lastFinished"].(string) != "" {
		t.Fatalf("initial: want seq 0 / empty name, got %v / %q", s0["finishSeq"], s0["lastFinished"])
	}

	// Finish alpha -> seq 1, name alpha.
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"Stop"}`))
	s1 := decode()
	if s1["finishSeq"].(float64) != 1 || s1["lastFinished"].(string) != "alpha" {
		t.Fatalf("after alpha: want seq 1 / alpha, got %v / %q", s1["finishSeq"], s1["lastFinished"])
	}

	// Finish beta -> seq 2, name beta.
	a.handleNotify([]byte(`{"session_id":"s2","cwd":"/tmp/b","hook_event_name":"Stop"}`))
	s2 := decode()
	if s2["finishSeq"].(float64) != 2 || s2["lastFinished"].(string) != "beta" {
		t.Fatalf("after beta: want seq 2 / beta, got %v / %q", s2["finishSeq"], s2["lastFinished"])
	}
}

// overlayCalls counts the overlay entry points for exclusivity tests. Swap the
// seam vars, run, restore.
//
// shows counts applyAvatarKindFn, which is now the single "raise the window"
// call: the old separate opacity/config-push counters folded into it, because
// overlay_wire.go owns those steps internally and they are no longer
// observable from outside the seam.
type overlayCalls struct{ shows, closes int }

func captureOverlay(t *testing.T) *overlayCalls {
	t.Helper()
	c := &overlayCalls{}
	oClose, oApply := overlayCloseFn, applyAvatarKindFn
	overlayCloseFn = func() { c.closes++ }
	applyAvatarKindFn = func(*App, CompanionConfig) { c.shows++ }
	t.Cleanup(func() { overlayCloseFn, applyAvatarKindFn = oClose, oApply })
	return c
}

func TestSetCompanionKindRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()
	captureOverlay(t)

	// Establish a known, non-default baseline FIRST via a legitimate call.
	// This matters for the assertion below: a.companion.Kind is the zero
	// value "" until something sets it, and GetCompanionKind() normalizes
	// both "" and "hologram" to the same KindOff — so asserting only
	// "!= KindOff" from a never-configured App cannot distinguish "nothing
	// was ever persisted" from "the rejected value got persisted and then
	// coincidentally clamped to off". Starting from KindPanel (not KindOff)
	// makes any deviation, including a silent snap-to-off, a visible failure.
	if err := a.SetCompanionKind(KindPanel); err != nil {
		t.Fatal(err)
	}

	if err := a.SetCompanionKind("hologram"); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
	// GetCompanionKind() NORMALIZES anything unrecognised to KindOff (that is
	// its whole job — see its doc comment), so a comparison against KindOff
	// through that getter proves nothing about what SetCompanionKind actually
	// persisted. Assert on the raw stored value instead — in memory AND on
	// disk — so a leak of the rejected string (or an incorrect fallback to
	// off) past validation is visible.
	if got := a.GetCompanionConfig().Kind; got != KindPanel {
		t.Fatalf("a rejected kind must leave the in-memory config exactly as it was, got %q want %q", got, KindPanel)
	}
	if got := loadCompanionConfig().Kind; got != KindPanel {
		t.Fatalf("a rejected kind must leave the on-disk config exactly as it was, got %q want %q", got, KindPanel)
	}
}

// payloadEmitter records event names AND payloads (fakeEmitter in app_test.go
// keeps names only). app_test.go has no public override — it is published
// as-is — so this distinct type is just about test-data needs, not about
// avoiding an edit to a mirrored file.
type payloadEmitter struct {
	mu   sync.Mutex
	seen []struct {
		name string
		data []any
	}
}

func (p *payloadEmitter) Emit(event string, data ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, struct {
		name string
		data []any
	}{event, data})
}

func (p *payloadEmitter) last(name string) (CompanionState, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.seen) - 1; i >= 0; i-- {
		if p.seen[i].name == name && len(p.seen[i].data) == 1 {
			st, ok := p.seen[i].data[0].(CompanionState)
			return st, ok
		}
	}
	return CompanionState{}, false
}

func TestCompanionStatePerSession(t *testing.T) {
	a := NewApp()
	a.notifier = &fakeNotifier{}
	pe := &payloadEmitter{}
	a.emitter = pe
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "alpha", Cwd: "/tmp/a", Status: "active", StatusSinceMs: 1000},
		"@2": {Name: "beta", Cwd: "/tmp/b", Status: "active", StatusSinceMs: 2000},
	}
	a.current = "@2"

	st := a.CompanionState()
	if len(st.Sessions) != 2 || st.Sessions[0].WindowID != "@1" || st.Sessions[1].WindowID != "@2" {
		t.Fatalf("sessions must be present and ordered by window id: %+v", st.Sessions)
	}
	if st.Selected != "@2" || st.Running != 2 || st.Finished != 0 {
		t.Fatalf("selection/counts wrong: %+v", st)
	}
	if st.Sessions[0].LastFinishMs != 0 || st.Sessions[0].ErrorMs != 0 {
		t.Fatalf("unfinished session must carry zero timestamps: %+v", st.Sessions[0])
	}

	// A BACKGROUND session finishing must move only that session's fields.
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"Stop"}`))
	st = a.CompanionState()
	if st.Sessions[0].Status != "finished" || st.Sessions[0].LastFinishMs == 0 {
		t.Fatalf("finished session lacks per-session finish stamp: %+v", st.Sessions[0])
	}
	if st.Sessions[0].StatusSinceMs != st.Sessions[0].LastFinishMs {
		t.Errorf("StatusSinceMs must move with the transition: %+v", st.Sessions[0])
	}
	if st.Sessions[1].Status != "active" || st.Sessions[1].LastFinishMs != 0 {
		t.Fatalf("a background finish must not touch the selected session: %+v", st.Sessions[1])
	}
	if st.Running != 1 || st.Finished != 1 || st.FinishSeq != 1 || st.LastFinished != "alpha" {
		t.Fatalf("aggregate wrong: %+v", st)
	}

	// ...and it must arrive as an event, not only by polling.
	got, ok := pe.last("companion:state")
	if !ok {
		t.Fatal("no companion:state event carrying a CompanionState was emitted")
	}
	if got.FinishSeq != 1 || len(got.Sessions) != 2 {
		t.Fatalf("event payload wrong: %+v", got)
	}
}

func TestCompanionStateEventOnKill(t *testing.T) {
	a := NewApp()
	pe := &payloadEmitter{}
	a.emitter = pe
	a.host = &captureHost{}
	a.sessions = map[string]*sessionRec{"@1": {Name: "x", Cwd: "/tmp/x", Status: "active"}}
	if err := a.KillSession("@1"); err != nil {
		t.Fatal(err)
	}
	got, ok := pe.last("companion:state")
	if !ok {
		t.Fatal("KillSession did not emit companion:state")
	}
	if len(got.Sessions) != 0 {
		t.Fatalf("killed session still present: %+v", got.Sessions)
	}
}

// The overlay's flat poll payload must not change shape (§3 boundary rule).
func TestLegacyOverlayStateJSONUnchanged(t *testing.T) {
	a := NewApp()
	a.setCompanionCountsForTest(2, 1, true, false)
	var got map[string]any
	if err := json.Unmarshal([]byte(a.companionStateJSON()), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"running", "finished", "awaiting", "error", "quotaHigh", "finishSeq", "lastFinished"} {
		if _, ok := got[k]; !ok {
			t.Errorf("legacy overlay payload lost key %q", k)
		}
	}
	if got["running"].(float64) != 2 || got["finished"].(float64) != 1 || got["awaiting"] != true {
		t.Fatalf("legacy payload wrong: %v", got)
	}
}

// pushCompanion must feed the desktop overlay and the deck from ONE
// companionSnapshot call, not two: companionSnapshot takes a.mu and sorts
// every session, so calling it twice per push means a concurrent finish
// landing between the two calls could feed the overlay and the deck two
// DIFFERENT states for what was meant to be a single, atomic push (it also
// silently doubles the per-push sort). companionSnapshotCallsForTest is
// incremented inside companionSnapshot itself, so this counts the real
// number of snapshots taken, not just the number of call sites in the
// source.
func TestPushCompanionTakesExactlyOneSnapshot(t *testing.T) {
	a := NewApp()
	a.setCompanionCountsForTest(1, 1, true, false)
	companionSnapshotCallsForTest.Store(0)

	a.pushCompanion()

	if n := companionSnapshotCallsForTest.Load(); n != 1 {
		t.Fatalf("pushCompanion took %d snapshots, want exactly 1", n)
	}
}

// Selecting a session is a display concern. It must not rewrite session state:
// "active" is sticky until the Stop hook, so relabelling on selection destroys
// the only record that the session had finished, and makes the room's
// "awaiting" state unreachable for any session you click into (§5.6).
func TestSelectSessionDoesNotRewriteStatus(t *testing.T) {
	a := NewApp()
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "alpha", Cwd: "/tmp/a", Status: "active", StatusSinceMs: 1000},
	}
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"Stop"}`))
	finishedAt := a.sessions["@1"].LastFinishMs

	if err := a.SelectSession("@1"); err != nil {
		t.Fatal(err)
	}
	if got := a.sessions["@1"].Status; got != "finished" {
		t.Errorf("Status = %q after selecting a finished session, want \"finished\"", got)
	}
	if got := a.sessions["@1"].LastFinishMs; got != finishedAt {
		t.Errorf("LastFinishMs = %d, want it untouched at %d", got, finishedAt)
	}
	if a.current != "@1" {
		t.Errorf("current = %q, want @1 — selection must still select", a.current)
	}
	// Selecting an unknown window is still a no-op, not a panic.
	if err := a.SelectSession("@nope"); err != nil {
		t.Fatal(err)
	}
}

// TestOverlaySelectAcknowledgesThenReassertsOnRefinish is the regression test
// for the gap SelectSession's Status-preservation opened: the Stop hook fires
// on every assistant turn, so a session's Status is "finished" for the rest
// of the process's life, and legacyOverlayState used to derive Awaiting
// straight from that count — so merely looking at a finished session never
// cleared Awaiting; it latched true forever. SelectSession now records a
// display-only acknowledgement timestamp, kept separate from Status, and
// legacyOverlayState compares it against the session's LastFinishMs so a
// later re-finish (which bumps LastFinishMs again) is not permanently muted
// by an old ack.
func TestOverlaySelectAcknowledgesThenReassertsOnRefinish(t *testing.T) {
	a := NewApp()
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "alpha", Cwd: "/tmp/a", Status: "active"},
	}

	// 1. A session finishes -> overlay reports Awaiting.
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"Stop"}`))
	if got := a.legacyOverlayState(a.companionSnapshot()); !got.Awaiting || got.Finished != 1 {
		t.Fatalf("after finish: want awaiting=true finished=1, got %+v", got)
	}

	// 2. The user selects it -> Awaiting must clear. Status must stay
	// "finished" (SelectSession must never rewrite it).
	time.Sleep(2 * time.Millisecond) // guarantee the ack timestamp > the finish timestamp
	if err := a.SelectSession("@1"); err != nil {
		t.Fatal(err)
	}
	if got := a.legacyOverlayState(a.companionSnapshot()); got.Awaiting || got.Finished != 0 {
		t.Fatalf("after select/ack: want awaiting=false finished=0, got %+v", got)
	}
	if got := a.sessions["@1"].Status; got != "finished" {
		t.Fatalf("acknowledging must not rewrite Status, got %q", got)
	}

	// 3. That same session finishes AGAIN afterwards -> Awaiting true again.
	// Proves the ack is not a permanent mute.
	time.Sleep(2 * time.Millisecond)
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"Stop"}`))
	if got := a.legacyOverlayState(a.companionSnapshot()); !got.Awaiting || got.Finished != 1 {
		t.Fatalf("after re-finish: want awaiting=true finished=1, got %+v", got)
	}
}

// Launch and swap failures returned only to the Wails caller (§2.2 defect 12).
// reportError already reaches the deck as app:error, which is the seam the
// room latches — so the failure paths have to use it.
func TestLaunchAndSwapFailuresReportError(t *testing.T) {
	countErrors := func(fe *fakeEmitter) int {
		fe.mu.Lock()
		defer fe.mu.Unlock()
		n := 0
		for _, e := range fe.events {
			if e == "app:error" {
				n++
			}
		}
		return n
	}

	t.Run("launch", func(t *testing.T) {
		a := NewApp()
		fe := &fakeEmitter{}
		a.emitter = fe
		if err := a.loadConfigFrom("example.config.toml"); err != nil {
			t.Fatal(err)
		}
		a.host = &captureHost{}
		if _, err := a.LaunchSession("/tmp/p", "no-such-model", false); err == nil {
			t.Fatal("expected an error for an unknown model")
		}
		if countErrors(fe) != 1 {
			t.Fatalf("launch failure did not emit app:error: %v", fe.events)
		}
	})

	t.Run("swap", func(t *testing.T) {
		a := NewApp()
		fe := &fakeEmitter{}
		a.emitter = fe
		if err := a.loadConfigFrom("example.config.toml"); err != nil {
			t.Fatal(err)
		}
		a.host = &captureHost{}
		a.sessions = map[string]*sessionRec{"@1": {Name: "s", Cwd: "/tmp/z", Status: "active"}}
		if _, err := a.SwapModel("@1", "no-such-model"); err == nil {
			t.Fatal("expected an error for an unknown model")
		}
		if countErrors(fe) != 1 {
			t.Fatalf("swap failure did not emit app:error: %v", fe.events)
		}
	})

	// The unknown-model cases above never reach startSession at all. The two
	// paths that actually matter — startSession itself failing inside
	// LaunchSession, and the post-kill relaunch failing inside SwapModel
	// (the "conversation preserved" case, where a silent error is most
	// damaging because the old window is already gone) — were untested.
	t.Run("startSession fails inside LaunchSession", func(t *testing.T) {
		a := NewApp()
		fe := &fakeEmitter{}
		a.emitter = fe
		if err := a.loadConfigFrom("example.config.toml"); err != nil {
			t.Fatal(err)
		}
		a.host = &captureHost{launchErr: fmt.Errorf("tmux is unavailable")}
		if _, err := a.LaunchSession("/tmp/p", "claude-opus-4-8", false); err == nil {
			t.Fatal("expected startSession's Launch failure to propagate out of LaunchSession")
		}
		if countErrors(fe) != 1 {
			t.Fatalf("a startSession failure inside LaunchSession did not emit app:error: %v", fe.events)
		}
	})

	t.Run("swap relaunch fails after the old window is already killed", func(t *testing.T) {
		a := NewApp()
		fe := &fakeEmitter{}
		a.emitter = fe
		if err := a.loadConfigFrom("example.config.toml"); err != nil {
			t.Fatal(err)
		}
		ch := &captureHost{}
		a.host = ch
		a.sessions = map[string]*sessionRec{
			"@1": {Name: "s", Cwd: "/tmp/z", Model: "claude-opus-4-8", Provider: "anthropic", Status: "active"},
		}
		a.current = "@1"
		// Everything up to the relaunch succeeds (known model, no routing
		// preflight); only the Launch call inside the post-kill startSession
		// fails — the exact "conversation preserved" case the wrapped error
		// message describes.
		ch.launchErr = fmt.Errorf("tmux is unavailable")
		if _, err := a.SwapModel("@1", "claude-opus-4-8"); err == nil {
			t.Fatal("expected the post-kill relaunch failure to propagate")
		}
		if len(ch.killed) != 1 || ch.killed[0] != "@1" {
			t.Fatalf("the old window must still be killed before the relaunch is attempted: %v", ch.killed)
		}
		if _, ok := a.sessions["@1"]; ok {
			t.Error("the old session record must be gone even though the relaunch failed")
		}
		if countErrors(fe) != 1 {
			t.Fatalf("a post-kill relaunch failure did not emit app:error: %v", fe.events)
		}
	})
}

// TestDetectFreshInstall pins the "once, then never again for this path"
// property directly on the pure function, without going through startup()'s
// network/tmux side effects.
func TestDetectFreshInstall(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")

	if !detectFreshInstall(p) {
		t.Fatal("a path that has never existed must be a fresh install")
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if detectFreshInstall(p) {
		t.Fatal("once the file exists, the same path must no longer read as fresh")
	}
	// Re-checking an already-established install stays false — "exactly
	// once", not "once per call before the first write we happen to see".
	if detectFreshInstall(p) {
		t.Fatal("detectFreshInstall must not flip back to true on a later call")
	}
}

// TestCompanionSnapshotSurfacesFreshInstall proves a.freshInstall (set once at
// startup) actually reaches the wire via CompanionState/companionSnapshot —
// the only path the frontend's freshInstall fact has to it.
func TestCompanionSnapshotSurfacesFreshInstall(t *testing.T) {
	a := NewApp()
	a.sessions = map[string]*sessionRec{"@1": {Name: "alpha", Status: "active"}}

	if got := a.CompanionState().FreshInstall; got {
		t.Fatalf("freshInstall defaults false, got %v", got)
	}

	a.mu.Lock()
	a.freshInstall = true
	a.mu.Unlock()

	if got := a.CompanionState().FreshInstall; !got {
		t.Fatal("a.freshInstall = true must surface as CompanionState.FreshInstall")
	}
	// It is a whole-process-life flag, not a one-shot: it must still read
	// true on a later, unrelated snapshot (a session finishing, say).
	a.sessions["@1"].Status = "finished"
	if got := a.CompanionState().FreshInstall; !got {
		t.Fatal("freshInstall must stay true across snapshots for the rest of the process")
	}
}

func TestClampedDemotesAvatarWhenBuildHasNoOverlay(t *testing.T) {
	defer func(v bool) { avatarKindAvailable = v }(avatarKindAvailable)
	avatarKindAvailable = false

	got := CompanionConfig{Kind: KindAvatar}.clamped()
	if got.Kind != KindOff {
		t.Errorf("Kind = %q, want %q", got.Kind, KindOff)
	}
	if got.Enabled {
		t.Error("Enabled should mirror the demoted kind, not the requested one")
	}
}

func TestClampedKeepsAvatarWhenBuildHasOverlay(t *testing.T) {
	defer func(v bool) { avatarKindAvailable = v }(avatarKindAvailable)
	avatarKindAvailable = true

	if got := (CompanionConfig{Kind: KindAvatar}).clamped(); got.Kind != KindAvatar {
		t.Errorf("Kind = %q, want %q -- private behaviour must be unchanged", got.Kind, KindAvatar)
	}
}

// The migration path is the one a real user reaches: a companion.json written
// before Kind existed, with enabled:true, on darwin.
func TestMigrationCannotLandOnAvatarWhenBuildHasNoOverlay(t *testing.T) {
	defer func(v bool) { avatarKindAvailable = v }(avatarKindAvailable)
	defer func(v string) { companionHostOS = v }(companionHostOS)
	avatarKindAvailable = false
	companionHostOS = "darwin"

	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	if err := os.WriteFile(companionConfigPath(), []byte(`{"enabled":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadCompanionConfig(); got.Kind != KindOff {
		t.Errorf("Kind = %q, want %q", got.Kind, KindOff)
	}
}

func TestSetCompanionKindRefusesAvatarWhenBuildHasNoOverlay(t *testing.T) {
	defer func(v bool) { avatarKindAvailable = v }(avatarKindAvailable)
	avatarKindAvailable = false

	a := NewApp()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	if err := a.SetCompanionKind(KindAvatar); err == nil {
		t.Error("want an error, got nil -- the UI could otherwise select a kind this build cannot honour")
	}
}
