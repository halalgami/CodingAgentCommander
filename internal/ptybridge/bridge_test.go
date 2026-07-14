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
		t.Skip("tmux not installed")
	}
}

func TestAttachStreamsAndAcceptsInput(t *testing.T) {
	requireTmux(t)
	session := "ccc_bridge_test"
	inputFile := t.TempDir() + "/in.txt"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })

	// window0 prints a sentinel then idles
	if err := exec.Command("tmux", "new-session", "-d", "-s", session, "-x", "200", "-y", "50",
		"sh", "-c", "echo BRIDGE_SENTINEL; sleep 60").Run(); err != nil {
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

	time.Sleep(1500 * time.Millisecond)
	mu.Lock()
	streamed := strings.Contains(out.String(), "BRIDGE_SENTINEL")
	mu.Unlock()
	if !streamed {
		t.Error("did not stream window output")
	}

	// input path: new window running cat, becomes current; write reaches it
	_ = exec.Command("tmux", "new-window", "-t", session, "sh", "-c", "cat >> "+inputFile).Run()
	time.Sleep(500 * time.Millisecond)
	if _, err := b.Write([]byte("BRIDGE_INPUT\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	body, _ := os.ReadFile(inputFile)
	if !strings.Contains(string(body), "BRIDGE_INPUT") {
		t.Errorf("input did not reach pane; file=%q", string(body))
	}

	if err := b.Resize(30, 100); err != nil {
		t.Errorf("Resize: %v", err)
	}
}
