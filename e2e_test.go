package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Run with: COMMANDER_E2E=1 go test . -run TestE2ELaunchStreamsOverWS -v
// Spawns a real claude session in tmux and asserts its screen streams over the ws.
func TestE2ELaunchStreamsOverWS(t *testing.T) {
	if os.Getenv("COMMANDER_E2E") == "" {
		t.Skip("set COMMANDER_E2E=1 to run (spawns a real claude session)")
	}
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	a.startWS()
	if a.WSPort() == 0 {
		t.Fatal("ws server did not start")
	}

	dir := t.TempDir()
	// pick the first native model
	models := a.Config()
	if len(models) == 0 {
		t.Fatal("no native models")
	}
	s, err := a.LaunchSession(dir, models[0].ID, false)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	t.Cleanup(func() { _ = a.host.Kill(a.cfg.TmuxSession, s.WindowID) })
	// also clean up the whole session at the end
	t.Cleanup(func() { _ = killTmux(a.cfg.TmuxSession) })

	// connect to the ws and read; expect substantial bytes (claude's TUI)
	url := "ws://127.0.0.1:" + itoa(a.WSPort()) + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(8 * time.Second))
	total := 0
	for i := 0; i < 200; i++ {
		_, data, err := c.ReadMessage()
		if err != nil {
			break
		}
		total += len(data)
		if total > 200 { // claude paints a non-trivial screen
			return // success
		}
	}
	t.Fatalf("streamed only %d bytes; expected claude's screen", total)
}

// killTmux kills the named tmux session, ignoring any error (e.g. already gone).
func killTmux(s string) error {
	return exec.Command("tmux", "kill-session", "-t", s).Run()
}

// itoa formats an int as a string.
func itoa(i int) string {
	return strconv.Itoa(i)
}
