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

// TestSetGetOptionRoundTrip guards the psmux compatibility fix: psmux's
// `show-options -wv` does not emit @user-options, so GetOption reads them back
// via `display-message` format expansion instead. Set then Get must round-trip.
func TestSetGetOptionRoundTrip(t *testing.T) {
	requireTmux(t)
	h := NewExecHost()
	session := "commander_test_opt"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	w, err := h.Launch(LaunchSpec{
		SessionName: session, WindowName: "opt", Dir: paneTempDir(t, session),
		Command: []string{"pwsh", "-NoProfile", "-NoLogo", "-Command", "Start-Sleep 30"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// Unset reads empty.
	if v, _ := h.GetOption(w.ID, "@commander_rc"); v != "" {
		t.Errorf("unset option should be empty, got %q", v)
	}
	if err := h.SetOption(w.ID, "@commander_rc", "1"); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if v, err := h.GetOption(w.ID, "@commander_rc"); err != nil || v != "1" {
		t.Errorf("GetOption after set = %q err=%v, want %q", v, err, "1")
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
