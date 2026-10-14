package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

const waitingW2 = "W2 is waiting for you to answer a prompt — answer it, then try again"

func hookBody(t *testing.T, fields map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func permissionPrompt(t *testing.T, cwd string) []byte {
	return hookBody(t, map[string]string{
		"session_id": "s2", "cwd": cwd, "hook_event_name": "Notification",
		"notification_type": "permission_prompt", "message": "Permission required to run Bash command",
	})
}

func guardApp(t *testing.T) (*App, *captureHost, string, string) {
	t.Helper()
	a, h := newAttachApp(t)
	proj := t.TempDir()
	addSession(a, "@2", "W2", proj)
	return a, h, proj, touch(t, filepath.Join(proj, "a.pdf"))
}

func TestPermissionPromptBlocksInsert(t *testing.T) {
	a, h, proj, file := guardApp(t)
	a.handleNotify(permissionPrompt(t, proj))
	for _, insert := range []func(string, []string) (AttachResult, error){a.InsertPaths, a.InsertDroppedPaths} {
		if _, err := insert("@2", []string{file}); err == nil || err.Error() != waitingW2 {
			t.Errorf("err = %v, want %q", err, waitingW2)
		}
	}
	if len(h.pasted) != 0 {
		t.Errorf("pasted %q while a prompt was waiting", h.pasted)
	}
}

func TestQuestionDialogBlocksInsert(t *testing.T) {
	a, _, proj, file := guardApp(t)
	a.handleNotify(hookBody(t, map[string]string{"cwd": proj, "hook_event_name": "Notification", "notification_type": "elicitation_dialog"}))
	if _, err := a.InsertPaths("@2", []string{file}); err == nil || err.Error() != waitingW2 {
		t.Errorf("err = %v", err)
	}
}

func TestIdleNoticeDoesNotBlock(t *testing.T) {
	a, h, proj, file := guardApp(t)
	a.handleNotify(hookBody(t, map[string]string{
		"cwd": proj, "hook_event_name": "Notification",
		"notification_type": "idle_prompt", "message": "Claude is waiting for your input",
	}))
	if _, err := a.InsertPaths("@2", []string{file}); err != nil || len(h.pasted) != 1 {
		t.Errorf("idle notice blocked an insert: %v", err)
	}
}

// Claude Code builds that predate notification_type only send a message.
func TestNotificationWithoutTypeFallsBackToTheMessage(t *testing.T) {
	a, _, proj, file := guardApp(t)
	a.handleNotify(hookBody(t, map[string]string{"cwd": proj, "hook_event_name": "Notification", "message": "Claude is waiting for your input"}))
	if _, err := a.InsertPaths("@2", []string{file}); err != nil {
		t.Errorf("idle message blocked: %v", err)
	}
	a.handleNotify(hookBody(t, map[string]string{"cwd": proj, "hook_event_name": "Notification", "message": "Claude needs your permission to use Bash"}))
	if _, err := a.InsertPaths("@2", []string{file}); err == nil || err.Error() != waitingW2 {
		t.Errorf("permission message did not block: %v", err)
	}
}

func TestEachAnswerSignalClearsTheGuard(t *testing.T) {
	for _, ev := range []string{"UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		t.Run(ev, func(t *testing.T) {
			a, h, proj, file := guardApp(t)
			a.handleNotify(permissionPrompt(t, proj))
			a.handleNotify(hookBody(t, map[string]string{"session_id": "s2", "cwd": proj, "hook_event_name": ev}))
			if _, err := a.InsertPaths("@2", []string{file}); err != nil || len(h.pasted) != 1 {
				t.Errorf("after %s: err = %v, pasted = %d", ev, err, len(h.pasted))
			}
		})
	}
}

// Before this task every /notify body was a finish. Now four more events post
// to it; none of them may mark the session finished, notify or emit.
func TestGuardEventsDoNotMarkFinished(t *testing.T) {
	a, _, proj, _ := guardApp(t)
	fe := &fakeEmitter{}
	fn := &fakeNotifier{}
	a.emitter, a.notifier = fe, fn
	a.handleNotify(permissionPrompt(t, proj))
	for _, ev := range []string{"UserPromptSubmit", "PreToolUse", "PostToolUse"} {
		a.handleNotify(hookBody(t, map[string]string{"cwd": proj, "hook_event_name": ev}))
	}
	if st := a.sessions["@2"].Status; st != "active" {
		t.Errorf("status = %q, want active", st)
	}
	if len(fe.events) != 0 || fn.calls != 0 {
		t.Errorf("events = %v, notifies = %d; want none", fe.events, fn.calls)
	}
}

func TestGuardOnlyTouchesTheMatchingFolder(t *testing.T) {
	a, h, proj, _ := guardApp(t)
	other := t.TempDir()
	addSession(a, "@3", "W3", other)
	a.handleNotify(permissionPrompt(t, proj))
	if _, err := a.InsertPaths("@3", []string{touch(t, filepath.Join(other, "b.pdf"))}); err != nil || len(h.pasted) != 1 {
		t.Errorf("a prompt in W2 blocked W3: %v", err)
	}
}

func TestKilledAndRelaunchedSessionsAreNotWaiting(t *testing.T) {
	keyring.MockInit()
	a, h, proj, file := guardApp(t)
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	r := a.sessions["@2"]
	// A known session id keeps SwapModel from scanning the real ~/.claude/projects.
	r.Model, r.Provider, r.ClaudeSessionID = "claude-opus-4-8", "anthropic", "sid-2"
	a.handleNotify(permissionPrompt(t, proj))

	// Relaunch (swap) gives a fresh record under a new window id ("@sw" from captureHost).
	if _, err := a.SwapModel("@2", "claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.InsertPaths("@sw", []string{file}); err != nil || len(h.pasted) != 1 {
		t.Errorf("relaunched session still waiting: %v", err)
	}
	// Kill forgets the window entirely.
	a.handleNotify(permissionPrompt(t, proj))
	if err := a.KillSession("@sw"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.InsertPaths("@sw", []string{file}); err == nil || err.Error() != "no such session" {
		t.Errorf("killed session: err = %v", err)
	}
}

func sharedCwdApp(t *testing.T) (*App, *captureHost, string, string) {
	t.Helper()
	a, h := newAttachApp(t)
	proj := t.TempDir()
	addSession(a, "@1", "W1", proj)
	addSession(a, "@2", "W2", proj)
	return a, h, proj, touch(t, filepath.Join(proj, "a.pdf"))
}

func hook(t *testing.T, ev, sid, cwd string) []byte {
	return hookBody(t, map[string]string{"session_id": sid, "cwd": cwd, "hook_event_name": ev})
}

func blocked(a *App, id string, file string) bool {
	_, err := a.InsertPaths(id, []string{file})
	return err != nil && strings.Contains(err.Error(), "is waiting for you to answer a prompt")
}

// (a) Mapping unknown: a prompt in a shared folder blocks BOTH sessions.
func TestSharedCwdUnknownMappingBlocksBoth(t *testing.T) {
	a, _, proj, file := sharedCwdApp(t)
	a.handleNotify(permissionPrompt(t, proj)) // session_id "s2", neither record has an id, 2 recs: nothing learned
	if !blocked(a, "@1", file) || !blocked(a, "@2", file) {
		t.Error("an unmapped prompt must block both sessions in the folder")
	}
}

// (b) Both mapped: W1's tool use must not clear W2's prompt.
func TestSiblingInSameFolderCannotClearAPrompt(t *testing.T) {
	a, _, proj, file := sharedCwdApp(t)
	a.sessions["@1"].ClaudeSessionID, a.sessions["@2"].ClaudeSessionID = "s1", "s2"
	a.handleNotify(permissionPrompt(t, proj)) // from s2
	if blocked(a, "@1", file) || !blocked(a, "@2", file) {
		t.Fatal("only W2 should be blocked by its own prompt")
	}
	a.handleNotify(hook(t, "PreToolUse", "s1", proj))
	if !blocked(a, "@2", file) {
		t.Error("W1's PreToolUse cleared W2's prompt")
	}
	if blocked(a, "@1", file) {
		t.Error("W1 should not be blocked")
	}
	// (c) its own signal clears it
	a.handleNotify(hook(t, "PreToolUse", "s2", proj))
	if blocked(a, "@2", file) {
		t.Error("W2's own PreToolUse did not clear its prompt")
	}
}

// A lone session in its folder teaches the mapping, so a later sibling's
// events are told apart.
func TestHookEventsTeachTheSessionMapping(t *testing.T) {
	a, _, proj, _ := guardApp(t)
	a.handleNotify(hook(t, "PreToolUse", "learned", proj))
	if got := a.sessions["@2"].ClaudeSessionID; got != "learned" {
		t.Errorf("ClaudeSessionID = %q", got)
	}
}

// Builds that installed only the Stop hook sent no hook_event_name.
func TestPayloadWithoutEventNameIsAStop(t *testing.T) {
	a, _, proj, _ := guardApp(t)
	a.handleNotify(hookBody(t, map[string]string{"session_id": "s2", "cwd": proj}))
	if st := a.sessions["@2"].Status; st != "finished" {
		t.Errorf("status = %q, want finished", st)
	}
}

func TestAttachMediaRefusesNonImageRegistrations(t *testing.T) {
	a, _ := newAttachApp(t)
	f := touch(t, filepath.Join(t.TempDir(), "secret.txt"))
	a.attachMedia = map[string]string{"aaaaaaaaaaaaaaaaaaaaaaaa": f}
	rec := httptest.NewRecorder()
	a.serveAttachMedia(rec, httptest.NewRequest("GET", "http://wails.localhost/media/attach/aaaaaaaaaaaaaaaaaaaaaaaa", nil))
	if rec.Code != 404 {
		t.Errorf("non-image served %d", rec.Code)
	}
	// positive control: the same id with an image extension is served
	img := touch(t, filepath.Join(t.TempDir(), "ok.png"))
	a.attachMedia["aaaaaaaaaaaaaaaaaaaaaaaa"] = img
	rec = httptest.NewRecorder()
	a.serveAttachMedia(rec, httptest.NewRequest("GET", "http://wails.localhost/media/attach/aaaaaaaaaaaaaaaaaaaaaaaa", nil))
	if rec.Code != 200 {
		t.Errorf("image control served %d", rec.Code)
	}
}

// Stop(s1) in a shared folder must not hand s1 to the wrong record: map order
// is random, so repeat. With two unmapped records the Stop is ambiguous and
// must not map anything; the prompt that follows must then block BOTH.
func TestSharedFolderStopNeverMisattributesTheSession(t *testing.T) {
	for i := 0; i < 50; i++ {
		a, _, proj, file := sharedCwdApp(t)
		a.sessions["@1"].ClaudeSessionID = "s1" // W1 is mapped; W2 is not
		a.handleNotify(hook(t, "Stop", "s1", proj))
		if got := a.sessions["@2"].ClaudeSessionID; got != "" {
			t.Fatalf("iteration %d: W1's Stop mapped s1 onto W2 (%q)", i, got)
		}
		if a.sessions["@1"].Status != "finished" || a.sessions["@2"].Status != "active" {
			t.Fatalf("iteration %d: statuses W1=%q W2=%q", i, a.sessions["@1"].Status, a.sessions["@2"].Status)
		}
		a.handleNotify(hookBody(t, map[string]string{"session_id": "s1", "cwd": proj, "hook_event_name": "Notification", "notification_type": "permission_prompt"}))
		if !blocked(a, "@1", file) {
			t.Fatalf("iteration %d: W1's own prompt did not block W1", i)
		}
		if blocked(a, "@2", file) {
			t.Fatalf("iteration %d: W1's prompt blocked W2", i)
		}
	}
}

func TestPromptFromASubdirectoryBlocksTheSession(t *testing.T) {
	a, _, proj, file := guardApp(t)
	sub := filepath.Join(proj, "frontend", "src")
	a.handleNotify(permissionPrompt(t, sub)) // Claude cd'd; cwd is no longer the folder
	if !blocked(a, "@2", file) {
		t.Error("a prompt from a subdirectory did not block the session")
	}
	a.handleNotify(hook(t, "PreToolUse", "s2", sub))
	if blocked(a, "@2", file) {
		t.Error("the session's own PreToolUse from the subdirectory did not clear it")
	}
}

func promptFrom(t *testing.T, sid, cwd string) []byte {
	return hookBody(t, map[string]string{"session_id": sid, "cwd": cwd, "hook_event_name": "Notification", "notification_type": "permission_prompt"})
}

func TestDeepestAncestorWinsAndPrefixSiblingDoesNotMatch(t *testing.T) {
	a, _ := newAttachApp(t)
	root := t.TempDir()
	app, app2, nested := filepath.Join(root, "app"), filepath.Join(root, "app2"), filepath.Join(root, "app", "pkg")
	addSession(a, "@1", "W1", app)
	addSession(a, "@2", "W2", app2)
	addSession(a, "@3", "W3", nested)
	f := touch(t, filepath.Join(root, "x.pdf"))
	// /root/app/pkg/sub: deepest ancestor is W3 (not W1)
	a.handleNotify(promptFrom(t, "sA", filepath.Join(nested, "sub")))
	if !blocked(a, "@3", f) || blocked(a, "@1", f) || blocked(a, "@2", f) {
		t.Error("deepest ancestor should be the only one blocked")
	}
	// /root/app2/x must not match /root/app (shared string prefix)
	a.handleNotify(hook(t, "PreToolUse", "sA", filepath.Join(nested, "sub")))
	a.handleNotify(promptFrom(t, "sB", filepath.Join(root, "app2", "x")))
	if !blocked(a, "@2", f) || blocked(a, "@1", f) {
		t.Error("/p/app2 must match W2 and not W1 (/p/app)")
	}
	// unrelated folder matches nothing
	a.handleNotify(hook(t, "PreToolUse", "sB", filepath.Join(app2, "x")))
	a.handleNotify(promptFrom(t, "sC", filepath.Join(root, "appX")))
	if blocked(a, "@1", f) || blocked(a, "@2", f) || blocked(a, "@3", f) {
		t.Error("an unrelated folder blocked a session")
	}
}
