//go:build !windows

package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

func TestLaunchAppliesEnvAndDir(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_launch"
	// Ensure clean slate and cleanup.
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })

	dir := t.TempDir()
	out := filepath.Join(dir, "probe.txt")
	// Probe writes env + pwd to a file, then idles so the window stays alive.
	spec := LaunchSpec{
		SessionName: session,
		WindowName:  "probe",
		Dir:         dir,
		Env:         map[string]string{"ANTHROPIC_MODEL": "claude-opus-4-8"},
		Command:     []string{"sh", "-c", "printf '%s\\n%s\\n' \"$ANTHROPIC_MODEL\" \"$PWD\" > " + out + "; sleep 30"},
	}
	w, err := h.Launch(spec)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected non-empty window ID")
	}

	// Wait for the probe file.
	var body []byte
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			body = b
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	got := string(body)
	wantModel := "claude-opus-4-8\n"
	if len(got) < len(wantModel) || got[:len(wantModel)] != wantModel {
		t.Errorf("env not applied; probe wrote %q", got)
	}

	// List should contain the window.
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

	// Kill removes it.
	if err := h.Kill(session, w.ID); err != nil {
		t.Fatalf("Kill: %v", err)
	}
}

func TestRename(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_rename"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })

	w, err := h.Launch(LaunchSpec{
		SessionName: session, WindowName: "before", Dir: t.TempDir(),
		Command: []string{"sh", "-c", "sleep 30"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := h.Rename(w.ID, "after"); err != nil {
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
// every session answer as whichever model was launched first: env staged for
// one window must not survive into the next. Real tmux applies `-e` per window
// and never had the bug (psmux, on Windows, does), but the staging Launch now
// does for psmux's benefit is what could reintroduce it here.
func TestLaunchDoesNotLeakEnvBetweenWindows(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_leak"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })

	dir := t.TempDir()
	managed := []string{"ANTHROPIC_MODEL", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}
	probe := func(out string) []string {
		return []string{"sh", "-c",
			`printf '%s\n[%s]\n' "$ANTHROPIC_MODEL" "$ANTHROPIC_BASE_URL" > ` + out + `; sleep 30`}
	}

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

	routed := readProbe(t, routedOut, 2)
	if routed[0] != "glm-5.3" {
		t.Errorf("routed window model = %q, want glm-5.3", routed[0])
	}
	if routed[1] != "[http://localhost:65000]" {
		t.Errorf("routed window base url = %s", routed[1])
	}
}

// readProbe waits for a probe file to hold at least n lines and returns them
// trimmed.
func readProbe(t *testing.T, path string, n int) []string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(path); err == nil {
			lines := strings.Split(string(b), "\n")
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
