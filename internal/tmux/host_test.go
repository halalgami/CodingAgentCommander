package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
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
