//go:build windows

package wsterm

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
)

// On Windows the bridge attaches to the psmux `tmux` shim and panes run pwsh.
// The sentinel is a single space-free token so cursor-motion escapes psmux may
// interleave between words don't split it; the input path drives an interactive
// pwsh with a carriage return, exactly as xterm.js delivers a keypress.

func requireTmux(t *testing.T) {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux (psmux) not installed")
	}
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh (PowerShell 7+) not installed")
	}
}

func TestHandlerStreamsPtyOverWebsocket(t *testing.T) {
	requireTmux(t)
	session := "ccc_wsterm_test_win"
	killSession(session)
	t.Cleanup(func() { killSession(session) })
	if err := newSentinelSession(session, "WSTERM_SENTINEL"); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(Handler("", func() (*ptybridge.Bridge, error) {
		return ptybridge.Attach(session, 50, 200)
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_ = c.SetReadDeadline(time.Now().Add(6 * time.Second))
	var got strings.Builder
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			break
		}
		got.Write(data)
		if strings.Contains(got.String(), "WSTERM_SENTINEL") {
			return // success
		}
	}
	t.Fatalf("never received sentinel; got %q", got.String())
}

func TestHandlerDeliversBinaryInputToPane(t *testing.T) {
	requireTmux(t)
	session := "ccc_wsterm_input_win"
	killSession(session)
	t.Cleanup(func() { killSession(session) })
	inputFile := filepath.Join(t.TempDir(), "in.txt")
	if err := newInteractiveSession(session); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler("", func() (*ptybridge.Bridge, error) {
		return ptybridge.Attach(session, 50, 200)
	}))
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	time.Sleep(2500 * time.Millisecond) // interactive pwsh startup
	// Raw bytes to the attach client are keystrokes; the trailing \r is Enter.
	cmd := "Set-Content -Encoding ascii -Path '" + inputFile + "' -Value WSINPUT\r"
	if err := c.WriteMessage(websocket.BinaryMessage, []byte(cmd)); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(2 * time.Second)
	body, _ := os.ReadFile(inputFile)
	if !strings.Contains(string(body), "WSINPUT") {
		t.Errorf("binary ws input did not reach pane; file=%q", string(body))
	}
}

func hasTmux() bool { _, err := exec.LookPath("tmux"); return err == nil }

func killSession(s string) { _ = exec.Command("tmux", "kill-session", "-t", s).Run() }

func newSentinelSession(s, sentinel string) error {
	// -NoExit keeps the pane at an interactive prompt after printing the
	// sentinel, so the line stays on the screen buffer and is redrawn when the
	// attach client connects. (A detached `while($true)` loop pane dies in
	// psmux, and a one-shot non-interactive print may not survive on redraw.)
	return exec.Command("tmux", "new-session", "-d", "-s", s, "-x", "200", "-y", "50",
		"pwsh", "-NoProfile", "-NoLogo", "-NoExit", "-Command", "Write-Host "+sentinel).Run()
}

func newInteractiveSession(s string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", s, "-x", "200", "-y", "50",
		"pwsh", "-NoProfile", "-NoLogo", "-NoExit").Run()
}
