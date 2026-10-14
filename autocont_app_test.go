package main

import (
	"encoding/json"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
	"github.com/halalgami/CodingAgentCommander/internal/winstate"
	"github.com/zalando/go-keyring"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

// stopApp is one Commander session (@1, glm, /tmp/a) with http hooks ready.
func stopApp(t *testing.T) (*App, *fakeEmitter, *fakeNotifier) {
	t.Helper()
	a := NewApp()
	fe, fn := &fakeEmitter{}, &fakeNotifier{}
	a.emitter, a.notifier = fe, fn
	a.setHookHTTP(true)
	a.configPath = t.TempDir() + "/config.toml"
	a.cfg.Models = []config.Model{
		{ID: "glm", Label: "Ollama · glm-5.3", Provider: "ollama-cloud"},
		{ID: "claude-haiku-5-5", Label: "Haiku 5.5", Provider: config.ProviderAnthropic},
	}
	a.sessions = map[string]*sessionRec{"@1": {Name: "a", Cwd: "/tmp/a", Model: "glm", Status: "active", ClaudeSessionID: "s1", Tag: "t1"}}
	a.current = "@9"
	return a, fe, fn
}

func stopBody(sid, prompt, msg string) []byte {
	b, _ := json.Marshal(map[string]any{"session_id": sid, "cwd": "/tmp/a", "hook_event_name": "Stop",
		"prompt_id": prompt, "last_assistant_message": msg, "permission_mode": "default"})
	return b
}

func isBlock(t *testing.T, out []byte) bool {
	t.Helper()
	var r struct{ Decision, Reason string }
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("reply %q: %v", out, err)
	}
	return r.Decision == "block"
}

func TestStopHookBlocksAPause(t *testing.T) {
	a, fe, fn := stopApp(t)
	out := a.handleStopHook(stopBody("s1", "p1", "Now the implementation:"), "t1")
	if !isBlock(t, out) || !strings.Contains(string(out), "make the tool call") {
		t.Fatalf("reply = %s", out)
	}
	if a.sessions["@1"].Status == "finished" || fn.calls != 0 || slices.Contains(fe.events, "session:finished") {
		t.Fatalf("blocked stop finished the session: status %q notify %d events %v", a.sessions["@1"].Status, fn.calls, fe.events)
	}
	if got := a.SessionStats("@1").AutoContinues; got != 1 {
		t.Fatalf("AutoContinues = %d", got)
	}
}

func TestStopHookRealStopFinishes(t *testing.T) {
	a, fe, _ := stopApp(t)
	out := a.handleStopHook(stopBody("s1", "p1", "All done, tests pass."), "t1")
	if isBlock(t, out) || a.sessions["@1"].Status != "finished" || !slices.Contains(fe.events, "session:finished") {
		t.Fatalf("reply %s status %q events %v", out, a.sessions["@1"].Status, fe.events)
	}
}

func TestStopHookIgnoresForeignSession(t *testing.T) {
	a, _, _ := stopApp(t)
	b, _ := json.Marshal(map[string]any{"session_id": "x", "cwd": "/elsewhere", "hook_event_name": "Stop",
		"prompt_id": "p", "last_assistant_message": "Let me check:"})
	if out := a.handleStopHook(b, ""); isBlock(t, out) {
		t.Fatalf("foreign session nudged: %s", out)
	}
}

// A terminal `claude` in the same folder as one Commander session carries no
// tag: it must never be nudged, even though the folder match is unique.
func TestStopHookIgnoresSameFolderTerminal(t *testing.T) {
	a, _, _ := stopApp(t)
	if out := a.handleStopHook(stopBody("term", "p1", "Now the tests:"), ""); isBlock(t, out) {
		t.Fatalf("terminal session in the same folder nudged: %s", out)
	}
	if out := a.handleStopHook(stopBody("term", "p1", "Now the tests:"), "nope"); isBlock(t, out) {
		t.Fatalf("unknown tag nudged: %s", out)
	}
}

// Two Commander sessions in one folder: the tag says which one stopped.
func TestStopHookTwoSessionsSameFolderUsesTag(t *testing.T) {
	a, _, _ := stopApp(t)
	a.sessions["@2"] = &sessionRec{Name: "b", Cwd: "/tmp/a", Model: "claude-haiku-5-5", Status: "active", ClaudeSessionID: "s2", Tag: "t2"}
	if out := a.handleStopHook(stopBody("s2", "p1", "Let me check:"), "t2"); isBlock(t, out) {
		t.Fatal("native session (default off) nudged")
	}
	if out := a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1"); !isBlock(t, out) {
		t.Fatal("glm session not nudged")
	}
}

func TestStopHookOffForNativeByDefault(t *testing.T) {
	a, _, _ := stopApp(t)
	a.sessions["@1"].Model = "claude-haiku-5-5"
	if out := a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1"); isBlock(t, out) {
		t.Fatalf("native default nudged: %s", out)
	}
}

func TestStopHookReadsSettingAtStopTime(t *testing.T) {
	a, _, _ := stopApp(t)
	if err := a.SetModelAutoContinue("glm", false); err != nil {
		t.Fatal(err)
	}
	if out := a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1"); isBlock(t, out) {
		t.Fatal("nudged with the setting off")
	}
	if err := a.SetModelAutoContinue("glm", true); err != nil {
		t.Fatal(err)
	}
	if out := a.handleStopHook(stopBody("s1", "p2", "Let me check:"), "t1"); !isBlock(t, out) {
		t.Fatal("not nudged after turning it back on")
	}
	got := a.Models()
	if !got[0].AutoContinue || !got[0].AutoContinueSet || !got[0].AutoContinueReady {
		t.Fatalf("ModelDetail = %+v", got[0])
	}
	if got[1].AutoContinue || got[1].AutoContinueSet {
		t.Fatalf("native ModelDetail = %+v", got[1])
	}
}

func TestStopHookGuards(t *testing.T) {
	cases := map[string]map[string]any{
		"plan mode":       {"permission_mode": "plan"},
		"background task": {"background_tasks": []any{map[string]any{"id": "b1"}}},
		"no message":      {"last_assistant_message": nil},
	}
	for name, patch := range cases {
		a, _, _ := stopApp(t)
		m := map[string]any{"session_id": "s1", "cwd": "/tmp/a", "hook_event_name": "Stop", "prompt_id": "p",
			"last_assistant_message": "Let me check:", "permission_mode": "default"}
		for k, v := range patch {
			if v == nil {
				delete(m, k)
			} else {
				m[k] = v
			}
		}
		b, _ := json.Marshal(m)
		if out := a.handleStopHook(b, "t1"); isBlock(t, out) {
			t.Errorf("%s: nudged", name)
		}
	}
}

func TestStopHookGivesUpWhenStuck(t *testing.T) {
	a, fe, _ := stopApp(t)
	a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1")
	out := a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1")
	if isBlock(t, out) {
		t.Fatal("nudged twice with no tool run")
	}
	if !slices.Contains(fe.events, "session:autocont-gaveup") || a.sessions["@1"].Status != "finished" {
		t.Fatalf("events %v status %q", fe.events, a.sessions["@1"].Status)
	}
}

func TestPostToolUseCountsAsProgress(t *testing.T) {
	a, _, _ := stopApp(t)
	a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1")
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"PostToolUse"}`))
	if out := a.handleStopHook(stopBody("s1", "p1", "Now the tests:"), "t1"); !isBlock(t, out) {
		t.Fatal("not nudged after a tool ran")
	}
}

func TestUserPromptResetsCount(t *testing.T) {
	a, _, _ := stopApp(t)
	a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1")
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","hook_event_name":"UserPromptSubmit"}`))
	if got := a.SessionStats("@1").AutoContinues; got != 0 {
		t.Fatalf("AutoContinues after new prompt = %d", got)
	}
}

func TestStopEndpointToken(t *testing.T) {
	a, _, _ := stopApp(t)
	a.wsToken = "tok"
	h := a.stopHandler()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/stop?token=bad", strings.NewReader("{}")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad token code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stop?token=tok", strings.NewReader(string(stopBody("s1", "p1", "Let me check:"))))
	req.Header.Set("X-Commander-Tag", "t1")
	h(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || !isBlock(t, rec.Body.Bytes()) {
		t.Fatalf("code %d type %q body %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

func TestKillDropsTracker(t *testing.T) {
	a, _, _ := stopApp(t)
	a.handleStopHook(stopBody("s1", "p1", "Let me check:"), "t1")
	a.dropAutoCont("@1")
	if a.autocont.Count("s1") != 0 {
		t.Fatal("tracker kept a killed session")
	}
}

func TestLaunchSetsCommanderTag(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	m, _ := a.cfg.Model("claude-opus-4-8")
	info, err := a.startSession("/tmp/p", m, nil)
	if err != nil {
		t.Fatal(err)
	}
	tag := ch.launched[0].Env["COMMANDER_TAG"]
	if len(tag) != 16 || a.sessions[info.WindowID].Tag != tag {
		t.Fatalf("env tag %q, rec tag %q", tag, a.sessions[info.WindowID].Tag)
	}
	if rec, _ := a.windows.Get(info.WindowID); rec.Tag != tag {
		t.Fatalf("window state tag %q, want %q", rec.Tag, tag)
	}
	if _, err := a.startSession("/tmp/p", m, nil); err != nil {
		t.Fatal(err)
	}
	if ch.launched[1].Env["COMMANDER_TAG"] == tag {
		t.Fatal("two launches share a tag")
	}
}

func TestReconcileRestoresTag(t *testing.T) {
	a := NewApp()
	ch := &captureHost{windows: []tmux.WindowState{{ID: "@5", Name: "p", Cwd: "/tmp/p"}}}
	a.host = ch
	_ = a.windows.Set("@5", winstate.Record{Model: "glm", SessionID: "s5", Tag: "t5"})
	a.reconcile(a.reconcileWindows())
	if r := a.sessions["@5"]; r == nil || r.Tag != "t5" {
		t.Fatalf("rec = %+v", r)
	}
}

// Minors sweep: hookHTTP is written at startup while HTTP goroutines and the
// Models binding read it.
func TestHookHTTPIsSafeToReadConcurrently(t *testing.T) {
	a, _, _ := stopApp(t)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			a.setHookHTTP(i%2 == 0)
		}
		close(done)
	}()
	for i := 0; i < 200; i++ {
		_ = a.Models()
		a.handleStopHook(stopBody("s1", "p", "done."), "t1")
	}
	<-done
}

// Minors round 2 (phase 4 review): a killed session stayed in the live tracker
// (and kept emitting) until it timed out.
func TestKillForgetsTheLiveSession(t *testing.T) {
	a, _, _ := stopApp(t)
	a.host = &captureHost{}
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.Prompt, At: 1, SessionID: "s1"}}, false)
	if _, ok := a.ledger.tracker.Get("s1"); !ok {
		t.Fatal("setup: session not tracked")
	}
	if err := a.KillSession("@1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.ledger.tracker.Get("s1"); ok {
		t.Fatal("killed session still tracked")
	}
}
