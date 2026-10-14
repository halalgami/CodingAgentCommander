// Package hookmgr installs/removes Commander-owned Claude Code hooks that post
// each hook payload to Commander's local /notify endpoint (or, for an http Stop
// hook, to /stop, which answers it): Stop (a session
// finished a turn) plus the events that track whether a session is waiting for
// the user to answer a permission prompt.
package hookmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sentinel marks Commander's hook command so Remove/Install act only on our block.
// It is carried as a trailing `#` comment, which is a line comment in every shell
// Claude Code runs hooks under (sh, bash, PowerShell).
const Sentinel = "__commander_notify__"

// command builds the Stop-hook command line. Claude pipes the hook JSON to
// stdin; curl posts it to the notify endpoint. The token authenticates the post
// so a stray local process can't spoof session-finished events. The concrete
// syntax is per-OS: see command_unix.go / command_windows.go.

func load(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func save(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Commander never installs PermissionRequest: that event can allow or deny
// tools, and an observer must not be on that path.
//
// Events are the Claude Code hook events Commander subscribes to. Every one
// posts the same payload to /notify, where handleNotify dispatches on
// hook_event_name. Stop marks a finished turn. Notification marks a session
// as waiting when it is a permission/question prompt (filtered server-side on
// notification_type, so no matcher is set here). UserPromptSubmit, PreToolUse
// and PostToolUse are the earliest signals that the user answered, so they
// clear it. No matcher means "every occurrence" for all five events.
var Events = []string{"Stop", "Notification", "UserPromptSubmit", "PreToolUse", "PostToolUse"}

// withoutCommander returns groups minus Commander's own, in order.
func withoutCommander(groups []any) []any {
	kept := groups[:0:0]
	for _, g := range groups {
		if !isCommanderGroup(g) {
			kept = append(kept, g)
		}
	}
	return kept
}

func isCommanderGroup(group any) bool {
	g, _ := group.(map[string]any)
	inner, _ := g["hooks"].([]any)
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, Sentinel) {
			return true
		}
		if u, _ := hm["url"].(string); strings.Contains(u, Sentinel) {
			return true
		}
	}
	return false
}

// StopURL is the http Stop hook's target. The sentinel rides in the query so
// isCommanderGroup recognises the group by its url. 127.0.0.1, not localhost:
// the server listens on IPv4 only, and Claude Code's own HTTP client may try
// ::1 first where curl would have fallen back.
func StopURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/stop?token=%s&c=%s", port, token, Sentinel)
}

// TagEnv is the variable Commander sets on every session it launches. The http
// Stop hook sends it back as TagHeader, so /stop knows exactly which window
// stopped; a session Commander did not launch sends it empty.
const (
	TagEnv    = "COMMANDER_TAG"
	TagHeader = "X-Commander-Tag"
)

// Install adds (or refreshes) Commander's group under every event in Events,
// after the user's own groups, preserving everything else in the file. With
// httpStop, Stop is an http hook Commander answers synchronously (auto-continue);
// only pass it for a Claude Code that supports http hooks (autocont.HookReady).
func Install(settingsPath string, port int, token string, httpStop bool) error {
	m, err := load(settingsPath)
	if err != nil {
		return err
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		if _, present := m["hooks"]; present && m["hooks"] != nil {
			return fmt.Errorf("%s: \"hooks\" is not an object; leaving it untouched", settingsPath)
		}
		hooks = map[string]any{}
		m["hooks"] = hooks
	}
	// An event whose value is not an array is the user's (or a typo): leave it
	// untouched and skip only that event, install the rest, and report it.
	var skipped []string
	for _, ev := range Events {
		v, present := hooks[ev]
		groups, isArr := v.([]any)
		if present && v != nil && !isArr {
			skipped = append(skipped, ev)
			continue
		}
		hook := map[string]any{"type": "command", "command": command(port, token), "timeout": 5}
		if ev == "Stop" && httpStop {
			hook = map[string]any{"type": "http", "url": StopURL(port, token), "timeout": 5,
				"headers": map[string]any{TagHeader: "$" + TagEnv}, "allowedEnvVars": []any{TagEnv}}
		}
		hooks[ev] = append(withoutCommander(groups), map[string]any{"hooks": []any{hook}})
	}
	if err := save(settingsPath, m); err != nil {
		return err
	}
	if len(skipped) > 0 {
		return &SkippedError{Path: settingsPath, Events: skipped}
	}
	return nil
}

// SkippedError reports events Install did not subscribe because their value
// in the settings file is not an array. Everything else was installed.
type SkippedError struct {
	Path   string
	Events []string
}

func (e *SkippedError) Error() string {
	return fmt.Sprintf("%s: hooks.%s is not an array; skipped (left untouched)", e.Path, strings.Join(e.Events, ", hooks."))
}

// Remove deletes Commander's groups under EVERY event — not only the ones this
// build installs, so a group left by an older build is swept too. An event
// key is deleted only when removing Commander's groups emptied it; a key the
// user left empty stays.
func Remove(settingsPath string) error {
	m, err := load(settingsPath)
	if err != nil {
		return err
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	for ev, v := range hooks {
		groups, ok := v.([]any)
		if !ok {
			continue
		}
		kept := withoutCommander(groups)
		if len(kept) == len(groups) {
			continue
		}
		if len(kept) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = kept
		}
	}
	if len(hooks) == 0 {
		delete(m, "hooks")
	}
	return save(settingsPath, m)
}
