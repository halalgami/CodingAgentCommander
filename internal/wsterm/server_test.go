package wsterm

import (
	"net/http"
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

func TestHandlerRejectsBadToken(t *testing.T) {
	// No tmux needed: the token gate rejects before open() is ever called.
	srv := httptest.NewServer(Handler("secret", func() (*ptybridge.Bridge, error) {
		t.Fatal("open() must not run when the token is wrong")
		return nil, nil
	}))
	t.Cleanup(srv.Close)
	base := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	// Missing token → 403, no upgrade.
	if _, resp, err := websocket.DefaultDialer.Dial(base, nil); err == nil {
		t.Error("dial without token should fail")
	} else if resp == nil || resp.StatusCode != 403 {
		t.Errorf("want 403, got resp=%v err=%v", resp, err)
	}
	// Wrong token → 403.
	if _, resp, err := websocket.DefaultDialer.Dial(base+"?token=nope", nil); err == nil {
		t.Error("dial with wrong token should fail")
	} else if resp == nil || resp.StatusCode != 403 {
		t.Errorf("want 403, got resp=%v err=%v", resp, err)
	}
}

func TestAllowedOrigin(t *testing.T) {
	mk := func(origin string) *http.Request {
		r := httptest.NewRequest("GET", "http://127.0.0.1/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	allow := map[string]bool{
		"":                        true, // native client
		"wails://wails":           true,
		"http://localhost:34115":  true,
		"http://wails.localhost":  true,
		"http://127.0.0.1:5000":   true,
		"https://evil.example.com": false, // third-party page must be refused
		"http://10.0.0.5":          false,
	}
	for origin, want := range allow {
		if got := allowedOrigin(mk(origin)); got != want {
			t.Errorf("allowedOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

func hasTmux() bool { _, err := lookPath("tmux"); return err == nil }

func lookPath(name string) (string, error) { return exec.LookPath(name) }

func killSession(s string) { _ = exec.Command("tmux", "kill-session", "-t", s).Run() }

func newSession(s, cmd string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", s, "-x", "200", "-y", "50",
		"sh", "-c", cmd).Run()
}
