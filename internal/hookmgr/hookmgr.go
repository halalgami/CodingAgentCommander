// Package hookmgr installs/removes a Commander-owned Claude Code Stop hook that
// pings Commander's local /notify endpoint when a session finishes a turn.
package hookmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sentinel marks Commander's hook command so Remove/Install act only on our block.
const Sentinel = "__commander_notify__"

func command(port int, token string) string {
	// Claude pipes the hook JSON to stdin; curl posts it to the notify endpoint.
	// The sentinel comment lets us identify our own block. The token authenticates
	// the post so a stray local process can't spoof session-finished events.
	return fmt.Sprintf(
		"curl -s -m 2 -X POST 'http://localhost:%d/notify?token=%s' -H 'Content-Type: application/json' -d @- >/dev/null 2>&1 # %s",
		port, token, Sentinel)
}

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

// stopGroups returns the []any under hooks.Stop, creating the path as needed.
func stopGroups(m map[string]any) []any {
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	groups, _ := hooks["Stop"].([]any)
	return groups
}

func isCommanderGroup(group any) bool {
	g, _ := group.(map[string]any)
	inner, _ := g["hooks"].([]any)
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, Sentinel) {
			return true
		}
	}
	return false
}

// Install adds (or refreshes) Commander's Stop hook, preserving everything else.
func Install(settingsPath string, port int, token string) error {
	m, err := load(settingsPath)
	if err != nil {
		return err
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		m["hooks"] = hooks
	}
	groups, _ := hooks["Stop"].([]any)
	// drop any prior commander group, then append a fresh one
	kept := groups[:0:0]
	for _, g := range groups {
		if !isCommanderGroup(g) {
			kept = append(kept, g)
		}
	}
	kept = append(kept, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command(port, token), "timeout": 5},
		},
	})
	hooks["Stop"] = kept
	return save(settingsPath, m)
}

// Remove deletes only Commander's Stop hook block.
func Remove(settingsPath string) error {
	m, err := load(settingsPath)
	if err != nil {
		return err
	}
	groups := stopGroups(m)
	if groups == nil {
		return nil
	}
	kept := groups[:0:0]
	for _, g := range groups {
		if !isCommanderGroup(g) {
			kept = append(kept, g)
		}
	}
	hooks, _ := m["hooks"].(map[string]any)
	if len(kept) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = kept
	}
	if len(hooks) == 0 {
		delete(m, "hooks")
	}
	return save(settingsPath, m)
}
