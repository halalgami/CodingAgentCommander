package wsterm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
)

// Integration tests that spin up a real tmux/psmux session and stream it over a
// websocket live in server_unix_test.go and server_windows_test.go (the pane
// shell and probe commands differ by platform). The tests below are pure
// protocol/logic and run everywhere.

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
		"":                         true, // native client
		"wails://wails":            true,
		"http://localhost:34115":   true,
		"http://wails.localhost":   true,
		"http://127.0.0.1:5000":    true,
		"https://evil.example.com": false, // third-party page must be refused
		"http://10.0.0.5":          false,
	}
	for origin, want := range allow {
		if got := allowedOrigin(mk(origin)); got != want {
			t.Errorf("allowedOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}
