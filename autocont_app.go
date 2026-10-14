package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/halalgami/CodingAgentCommander/internal/autocont"
	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/hookmgr"
)

// AutoContGaveUp is the session:autocont-gaveup payload: auto-continue stopped
// nudging (why = "stuck" | "cap") and the user is back in charge.
type AutoContGaveUp struct {
	WindowID string `json:"windowID"`
	Model    string `json:"model"`
	Why      string `json:"why"`
}

var noDecision = []byte(`{}`)

// stopHandler serves the http Stop hook: same token as /notify, and the reply
// body is the hook decision Claude Code reads.
func (a *App) stopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.wsToken != "" && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(a.wsToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(a.handleStopHook(body, r.Header.Get(hookmgr.TagHeader)))
	}
}

// handleStopHook decides one Stop. tag is the X-Commander-Tag header: only the
// window launched with that tag can be nudged, so a terminal `claude` in the
// same folder (no tag) never is. A nudge keeps the session working: no
// finished status, notification or ledger Stop. Anything else runs the
// ordinary Stop path and replies {} (a normal stop).
func (a *App) handleStopHook(body []byte, tag string) []byte {
	var p hookPayload
	if json.Unmarshal(body, &p) != nil {
		return noDecision
	}
	a.learnSession(p)
	v, windowID, label := a.autoDecide(p, tag)
	if v == autocont.Block {
		a.setAwaiting(p, false)
		out, _ := json.Marshal(map[string]string{"decision": "block", "reason": autocont.Nudge})
		return out
	}
	if v == autocont.GaveUpStuck || v == autocont.GaveUpCap {
		why := "stuck"
		if v == autocont.GaveUpCap {
			why = "cap"
		}
		a.emitter.Emit("session:autocont-gaveup", AutoContGaveUp{WindowID: windowID, Model: label, Why: why})
	}
	a.handleNotify(body)
	return noDecision
}

// autoDecide applies the never-block guards, then the tracker. The window is
// the one launched with tag — never a folder match. The model setting is read
// now, so a toggle applies to the next pause.
func (a *App) autoDecide(p hookPayload, tag string) (autocont.Verdict, string, string) {
	if !a.hookHTTP.Load() || tag == "" || p.SessionID == "" || p.LastAssistantMessage == nil ||
		p.PermissionMode == "plan" || hasTasks(p.BackgroundTasks) {
		return autocont.Allow, "", ""
	}
	a.mu.Lock()
	var windowID, modelID string
	for wid, r := range a.sessions {
		if r.Tag == tag {
			windowID, modelID = wid, r.Model
		}
	}
	model, ok := a.cfg.Model(modelID)
	a.mu.Unlock()
	if windowID == "" {
		return autocont.Allow, "", ""
	}
	if !ok || !model.AutoContinueOn() {
		return autocont.Allow, "", ""
	}
	label := model.Label
	if label == "" {
		label = model.ID
	}
	return a.autocont.Decide(p.SessionID, p.PromptID, autocont.Detect(*p.LastAssistantMessage)), windowID, label
}

// hasTasks reports a non-empty background_tasks value (array or object).
func hasTasks(raw json.RawMessage) bool {
	switch s := string(raw); s {
	case "", "null", "[]", "{}":
		return false
	}
	return true
}

// setHookHTTP records whether the installed Claude Code takes the http Stop
// hook; startup writes it while HTTP goroutines and bindings may read it.
func (a *App) setHookHTTP(on bool) { a.hookHTTP.Store(on) }

// dropAutoCont forgets a window's tracker entry. Caller holds a.mu.
func (a *App) dropAutoCont(windowID string) {
	if r := a.sessions[windowID]; r != nil && r.ClaudeSessionID != "" {
		a.autocont.Drop(r.ClaudeSessionID)
	}
}

// SetModelAutoContinue stores an explicit per-model auto-continue setting.
func (a *App) SetModelAutoContinue(id string, on bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	models := append([]config.Model(nil), a.cfg.Models...)
	i := -1
	for j, m := range models {
		if m.ID == id {
			i = j
		}
	}
	if i < 0 {
		return fmt.Errorf("model %q not found", id)
	}
	v := on
	models[i].AutoContinue = &v
	next := a.cfg
	next.Models = models
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next
	return nil
}
