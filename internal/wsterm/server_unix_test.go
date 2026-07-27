//go:build !windows

package wsterm

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
}

func TestHandlerStreamsPtyOverWebsocket(t *testing.T) {
	requireTmux(t)
	session := "ccc_wsterm_test"
	killSession(session)
	t.Cleanup(func() { killSession(session) })
	if err := newSession(session, "echo WSTERM_SENTINEL; sleep 60"); err != nil {
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

	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
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
	session := "ccc_wsterm_input"
	killSession(session)
	t.Cleanup(func() { killSession(session) })
	inputFile := t.TempDir() + "/in.txt"
	if err := newSession(session, "cat >> "+inputFile); err != nil {
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
	time.Sleep(500 * time.Millisecond)
	if err := c.WriteMessage(websocket.BinaryMessage, []byte("WSINPUT\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(700 * time.Millisecond)
	body, _ := os.ReadFile(inputFile)
	if !strings.Contains(string(body), "WSINPUT") {
		t.Errorf("binary ws input did not reach pane; file=%q", string(body))
	}
}

func hasTmux() bool { _, err := exec.LookPath("tmux"); return err == nil }

func killSession(s string) { _ = exec.Command("tmux", "kill-session", "-t", s).Run() }

func newSession(s, cmd string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", s, "-x", "200", "-y", "50",
		"sh", "-c", cmd).Run()
}
