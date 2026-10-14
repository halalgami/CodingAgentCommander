//go:build !windows

package tmux

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// isolatedTmux points every tmux call in this test at a private server, so
// the test can neither see nor disturb the user's real sessions, and kills it
// afterwards. The socket dir is under /tmp, not t.TempDir(): macOS temp paths
// are long enough that "<dir>/tmux-<uid>/default" overflows the 104-byte unix
// socket limit. Cleanups run LIFO, so kill-server runs while the env still
// points at the private server.
func isolatedTmux(t *testing.T) {
	t.Helper()
	requireTmux(t)
	dir, err := os.MkdirTemp("/tmp", "cmdr-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", dir)
	t.Cleanup(func() {
		_ = tmuxCmd("kill-server").Run()
		_ = os.RemoveAll(dir)
	})
}

// launchRawReader starts a window whose program advertises bracketed paste
// (DECSET 2004), switches its tty to raw mode, and copies exactly n bytes of
// input to out. Raw mode matters: a canonical-mode reader leaves the tail in
// the line-discipline buffer and LOOKS like truncation (the spec's trap).
func launchRawReader(t *testing.T, h *ExecHost, out string, n int) string {
	t.Helper()
	w, err := h.Launch(LaunchSpec{
		SessionName: "commander_test_paste", WindowName: "reader", Dir: t.TempDir(),
		Command: []string{"sh", "-c", "printf '\\033[?2004h'; stty raw -echo; dd bs=1 count=" + strconv.Itoa(n) + " of=" + out + " 2>/dev/null; sleep 30"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	time.Sleep(400 * time.Millisecond) // let stty take effect before input arrives
	return w.ID
}

// TestPasteIsBracketedAndByteExact reads ONE byte more than the expected
// bracketed payload and then waits: if Paste ever pressed Enter (or appended
// anything), that extra byte arrives and the exact comparison fails.
func TestPasteIsBracketedAndByteExact(t *testing.T) {
	isolatedTmux(t)
	h := NewExecHost()
	payload := "-dash.png ./My\\ File\\ \\(1\\).png /tmp/ünï.pdf "
	want := "\x1b[200~" + payload + "\x1b[201~"
	out := filepath.Join(t.TempDir(), "got")
	id := launchRawReader(t, h, out, len(want)+1)

	if err := h.Paste(id, payload); err != nil {
		t.Fatalf("Paste: %v", err)
	}
	time.Sleep(1 * time.Second)
	got, _ := os.ReadFile(out)
	if string(got) != want {
		t.Errorf("received %q\nwant     %q", got, want)
	}
	if out, _ := tmuxCmd("list-buffers").Output(); strings.Contains(string(out), "commander-attach") {
		t.Errorf("paste buffer left behind; -d should delete it: %s", out)
	}
}

func TestProbePasteBuffersAgainstARealServer(t *testing.T) {
	isolatedTmux(t)
	if probePasteBuffers() {
		t.Fatal("probe succeeded with no tmux server running")
	}
	if err := tmuxCmd("new-session", "-d", "-s", "commander_test_probe", "sleep 30").Run(); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	if !probePasteBuffers() {
		t.Fatal("probe failed against a running tmux")
	}
	if err := tmuxCmd("show-buffer", "-b", "commander-probe").Run(); err == nil {
		t.Error("probe buffer left behind")
	}
}

func TestPasteWithoutBuffersRefusesAndTypesNothing(t *testing.T) {
	isolatedTmux(t)
	orig := probePasteBuffers
	probePasteBuffers = func() bool { return false }
	t.Cleanup(func() { probePasteBuffers = orig })

	h := NewExecHost()
	out := filepath.Join(t.TempDir(), "got")
	id := launchRawReader(t, h, out, 1)
	err := h.Paste(id, "./a.png ")
	if !errors.Is(err, ErrPasteUnsupported) || err.Error() != "inserting is not supported by this tmux" {
		t.Fatalf("Paste err = %v, want ErrPasteUnsupported", err)
	}
	time.Sleep(1 * time.Second)
	if got, _ := os.ReadFile(out); len(got) != 0 {
		t.Errorf("typed %q into the window; a refused insert must send nothing", got)
	}
}

func TestPasteErrorCarriesTmuxStderr(t *testing.T) {
	isolatedTmux(t)
	if err := tmuxCmd("new-session", "-d", "-s", "commander_test_err", "sleep 30").Run(); err != nil {
		t.Fatalf("new-session: %v", err)
	}
	err := NewExecHost().Paste("@999", "x ")
	if err == nil {
		t.Fatal("paste into a missing window succeeded")
	}
	// Measured on tmux 3.7b: "can't find window: @999" on stderr, exit 1.
	if !strings.HasPrefix(err.Error(), "tmux paste-buffer: can't find window: @999") {
		t.Errorf("err = %q, want tmux's stderr after the tmux paste-buffer prefix", err)
	}
}

// A tmux whose buffers do not round-trip the text must fail the probe: the
// fake binary lets set-buffer/delete-buffer succeed but show-buffer lie.
func TestProbeRejectsMismatchedBufferContent(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in show-buffer) printf 'something else';; *) exit 0;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if probePasteBuffers() {
		t.Error("probe accepted a buffer that did not round-trip")
	}
	if err := NewExecHost().Paste("@1", "x "); !errors.Is(err, ErrPasteUnsupported) {
		t.Errorf("Paste err = %v, want ErrPasteUnsupported", err)
	}
}
