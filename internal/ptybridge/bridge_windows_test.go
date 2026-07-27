//go:build windows

package ptybridge

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux (psmux) not installed")
	}
}

// TestAttachStreamsAndAcceptsInput exercises the ConPTY bridge against a real
// psmux server: it must stream a pane's output to Read and deliver Write bytes
// as keystrokes into the active pane. pwsh is the shell here (psmux's Claude
// Code integration requires PowerShell 7+), so the probe commands are pwsh.
func TestAttachStreamsAndAcceptsInput(t *testing.T) {
	requireTmux(t)
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh (PowerShell 7+) not installed")
	}
	session := "ccc_bridge_test_win"
	inputFile := t.TempDir() + `\in.txt`
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })

	// window0 prints a sentinel then stays at an interactive prompt (-NoExit) so
	// the line survives on the screen buffer and is redrawn when Attach connects.
	// (A detached `while($true)` loop pane dies in psmux, taking the server down.)
	if err := exec.Command("tmux", "new-session", "-d", "-s", session, "-x", "200", "-y", "50",
		"pwsh", "-NoProfile", "-NoLogo", "-NoExit", "-Command", "Write-Host BRIDGE_SENTINEL").Run(); err != nil {
		t.Fatal(err)
	}

	b, err := Attach(session, 50, 200)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	var mu sync.Mutex
	var out strings.Builder
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := b.Read(buf)
			if n > 0 {
				mu.Lock()
				out.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(2500 * time.Millisecond)
	mu.Lock()
	streamed := strings.Contains(out.String(), "BRIDGE_SENTINEL")
	mu.Unlock()
	if !streamed {
		t.Error("did not stream window output")
	}

	// input path: a new window running interactive pwsh becomes current; bytes
	// written to the bridge are keystrokes, and a trailing \r is Enter — exactly
	// how xterm.js delivers a keypress. The typed command must create the file.
	_ = exec.Command("tmux", "new-window", "-t", session,
		"pwsh", "-NoProfile", "-NoLogo", "-NoExit").Run()
	time.Sleep(2500 * time.Millisecond) // interactive pwsh startup
	if _, err := b.Write([]byte("Set-Content -Encoding ascii -Path '" + inputFile + "' -Value BRIDGE_INPUT\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	time.Sleep(2 * time.Second)
	body, _ := os.ReadFile(inputFile)
	if !strings.Contains(string(body), "BRIDGE_INPUT") {
		t.Errorf("input did not reach pane; file=%q", string(body))
	}

	if err := b.Resize(30, 100); err != nil {
		t.Errorf("Resize: %v", err)
	}
}
