//go:build windows

package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// On Windows the ExecHost drives the psmux `tmux` shim and panes run pwsh, so
// the probe/idle commands are PowerShell rather than sh. Behaviour under test —
// -e env injection, -c start dir, list, rename, kill — is identical.

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux (psmux) not installed")
	}
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh (PowerShell 7+) not installed")
	}
}

// paneTempDir returns a temp dir to use as a pane's start directory, plus a
// cleanup that first kills the session then removes the dir with retries.
// Windows refuses to delete a directory that is a live process's current
// directory, and psmux releases the pane process's handle asynchronously after
// kill-session — so t.TempDir's immediate RemoveAll would race and fail.
func paneTempDir(t *testing.T, session string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "commander_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", session).Run()
		for i := 0; i < 40; i++ {
			if os.RemoveAll(dir) == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	return dir
}

func TestLaunchAppliesEnvAndDir(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_launch"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	dir := paneTempDir(t, session)
	out := filepath.Join(dir, "probe.txt")
	// Probe writes the -e-provided model then the cwd, then idles so the window
	// stays alive. ascii encoding avoids a UTF-8 BOM in the file.
	script := "Set-Content -Encoding ascii -Path '" + out + "' -Value $env:ANTHROPIC_MODEL; " +
		"Add-Content -Encoding ascii -Path '" + out + "' -Value (Get-Location).Path; Start-Sleep 30"
	spec := LaunchSpec{
		SessionName: session,
		WindowName:  "probe",
		Dir:         dir,
		Env:         map[string]string{"ANTHROPIC_MODEL": "claude-opus-4-8"},
		Command:     []string{"pwsh", "-NoProfile", "-NoLogo", "-Command", script},
	}
	w, err := h.Launch(spec)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected non-empty window ID")
	}

	var body []byte
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			body = b
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// First line should be the injected model (tolerate CRLF).
	firstLine := strings.TrimSpace(strings.SplitN(string(body), "\n", 2)[0])
	if firstLine != "claude-opus-4-8" {
		t.Errorf("env not applied; probe wrote %q", string(body))
	}

	ws, err := h.List(session)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, x := range ws {
		if x.ID == w.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("List did not contain launched window %s; got %+v", w.ID, ws)
	}

	if err := h.Kill(session, w.ID); err != nil {
		t.Fatalf("Kill: %v", err)
	}
}

func TestRename(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_rename"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	w, err := h.Launch(LaunchSpec{
		SessionName: session, WindowName: "before", Dir: paneTempDir(t, session),
		Command: []string{"pwsh", "-NoProfile", "-NoLogo", "-Command", "Start-Sleep 30"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := h.Rename(session, w.ID, "after"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	ws, _ := h.List(session)
	found := false
	for _, x := range ws {
		if x.ID == w.ID && x.Name == "after" {
			found = true
		}
	}
	if !found {
		t.Errorf("window not renamed; got %+v", ws)
	}
}

// TestLaunchDoesNotLeakEnvBetweenWindows guards the fix for the leak that made
// every session answer as whichever model was launched first. psmux ignores
// `-e` on new-window and, on the new-session that starts the server, copies it
// into the session and server-wide environments — so a routed launch's
// ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN were inherited by every later
// window, quietly sending native Anthropic sessions to the LiteLLM proxy.
//
// Window 1 launches routed, window 2 native. Window 2 must see its own model
// and no base URL at all.
func TestLaunchDoesNotLeakEnvBetweenWindows(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_leak"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	dir := paneTempDir(t, session)
	managed := []string{"ANTHROPIC_MODEL", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}
	probe := func(out string) []string {
		return []string{"pwsh", "-NoProfile", "-NoLogo", "-Command",
			"Set-Content -Encoding ascii -Path '" + out + "' -Value $env:ANTHROPIC_MODEL; " +
				"Add-Content -Encoding ascii -Path '" + out + "' -Value \"[$env:ANTHROPIC_BASE_URL]\"; " +
				"Start-Sleep 30"}
	}

	// Routed launch — creates the session, and used to poison it.
	routedOut := filepath.Join(dir, "routed.txt")
	if _, err := h.Launch(LaunchSpec{
		SessionName: session, WindowName: "routed", Dir: dir,
		Env: map[string]string{
			"ANTHROPIC_MODEL":      "glm-5.3",
			"ANTHROPIC_BASE_URL":   "http://localhost:65000",
			"ANTHROPIC_AUTH_TOKEN": "sk-routed",
		},
		ClearEnv: managed, Command: probe(routedOut),
	}); err != nil {
		t.Fatalf("Launch routed: %v", err)
	}

	// Native launch — sets only the model, and must not inherit the proxy.
	nativeOut := filepath.Join(dir, "native.txt")
	if _, err := h.Launch(LaunchSpec{
		SessionName: session, WindowName: "native", Dir: dir,
		Env:      map[string]string{"ANTHROPIC_MODEL": "claude-opus-4-8"},
		ClearEnv: managed, Command: probe(nativeOut),
	}); err != nil {
		t.Fatalf("Launch native: %v", err)
	}

	lines := readProbe(t, nativeOut, 2)
	if lines[0] != "claude-opus-4-8" {
		t.Errorf("native window model = %q, want claude-opus-4-8", lines[0])
	}
	if lines[1] != "[]" {
		t.Errorf("native window inherited ANTHROPIC_BASE_URL = %s; the routed launch leaked into it", lines[1])
	}

	// The routed window still got its own environment.
	routed := readProbe(t, routedOut, 2)
	if routed[0] != "glm-5.3" {
		t.Errorf("routed window model = %q, want glm-5.3", routed[0])
	}
	if routed[1] != "[http://localhost:65000]" {
		t.Errorf("routed window base url = %s", routed[1])
	}
}

// readProbe waits for a probe file to hold at least n lines and returns them
// trimmed (panes write CRLF).
func readProbe(t *testing.T, path string, n int) []string {
	t.Helper()
	for i := 0; i < 100; i++ {
		b, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
			if len(lines) >= n {
				for j := range lines {
					lines[j] = strings.TrimSpace(lines[j])
				}
				return lines
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("probe %s never reached %d lines", path, n)
	return nil
}

// TestWindowTargetedCommandsHitTheRightWindow guards the fix for psmux's
// window-id handling. psmux does not resolve ids as -t targets: select-window
// strips the "@" and uses the rest as an *index*, kill-window resolved the id
// against another session entirely (killing the wrong session's window), and
// rename-window silently did nothing. Ids and indexes never line up — ids start
// at @1 and are server-wide, indexes are per-session — so targeting one when you
// meant the other lands on a different window or none.
func TestWindowTargetedCommandsHitTheRightWindow(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_target"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	dir := paneTempDir(t, session)
	idle := []string{"pwsh", "-NoProfile", "-NoLogo", "-Command", "Start-Sleep 60"}
	var ids []string
	for _, name := range []string{"first", "second", "third"} {
		w, err := h.Launch(LaunchSpec{
			SessionName: session, WindowName: name, Dir: dir, Command: idle,
		})
		if err != nil {
			t.Fatalf("Launch %s: %v", name, err)
		}
		ids = append(ids, w.ID)
	}

	// Every id resolves, to a distinct target, and to something other than the
	// bare id — otherwise the test could pass while still targeting ids.
	seen := map[string]bool{}
	for _, id := range ids {
		target := WindowTarget(session, id)
		if target == "" {
			t.Fatalf("WindowTarget(%s) did not resolve", id)
		}
		if seen[target] {
			t.Fatalf("WindowTarget(%s) = %q, already used by another window", id, target)
		}
		seen[target] = true
		if target == id {
			t.Errorf("WindowTarget(%s) returned the raw id; expected session:index", id)
		}
	}
	if WindowTarget(session, "@99999") != "" {
		t.Error("WindowTarget resolved an id that does not exist")
	}

	// Rename the third window by id: only that one may change.
	if err := h.Rename(session, ids[2], "renamed"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	names := namesByID(t, h, session)
	if names[ids[2]] != "renamed" {
		t.Errorf("third window not renamed; got %q", names[ids[2]])
	}
	if names[ids[0]] != "first" || names[ids[1]] != "second" {
		t.Errorf("rename hit the wrong window: %v", names)
	}

	// Kill the middle window by id: the other two must survive.
	if err := h.Kill(session, ids[1]); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	names = namesByID(t, h, session)
	if _, alive := names[ids[1]]; alive {
		t.Errorf("Kill did not remove %s; windows = %v", ids[1], names)
	}
	if len(names) != 2 || names[ids[0]] != "first" || names[ids[2]] != "renamed" {
		t.Errorf("Kill removed the wrong window; windows = %v", names)
	}

	// A window that has already gone is an error, not a silent no-op that
	// could land on whatever now occupies that index.
	if err := h.Kill(session, ids[1]); err == nil {
		t.Error("Kill of a dead window should error rather than target something else")
	}
}

// namesByID maps window id to name for a session.
func namesByID(t *testing.T, h *ExecHost, session string) map[string]string {
	t.Helper()
	ws, err := h.List(session)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := map[string]string{}
	for _, w := range ws {
		out[w.ID] = w.Name
	}
	return out
}
