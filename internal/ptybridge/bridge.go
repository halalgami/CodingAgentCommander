// Package ptybridge runs `tmux attach` inside a pty so a session can be
// streamed to a terminal emulator (xterm.js) and driven by it.
package ptybridge

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// Bridge is a pty attached to a tmux session. It is an io.ReadWriteCloser:
// Read yields the session's screen bytes, Write sends keystrokes.
type Bridge struct {
	session string
	f       *os.File
	cmd     *exec.Cmd
}

// Attach opens a pty running `tmux attach` for session, sized rows x cols,
// with the tmux status bar disabled for a clean embed.
func Attach(session string, rows, cols uint16) (*Bridge, error) {
	// Hide chrome; ignore error if the session has none yet.
	_ = exec.Command("tmux", "set-option", "-t", session, "status", "off").Run()

	cmd := exec.Command("tmux", "attach-session", "-t", session)
	// GUI-launched apps have no TERM; without one the attach client dies with
	// "open terminal failed: terminal does not support clear" and the embedded
	// terminal stays blank. xterm.js is xterm-256color compatible.
	cmd.Env = os.Environ()
	if os.Getenv("TERM") == "" {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, fmt.Errorf("pty attach %s: %w", session, err)
	}
	return &Bridge{session: session, f: f, cmd: cmd}, nil
}

// Read implements io.Reader (screen bytes from the session).
func (b *Bridge) Read(p []byte) (int, error) { return b.f.Read(p) }

// Write implements io.Writer (keystrokes into the active pane).
func (b *Bridge) Write(p []byte) (int, error) { return b.f.Write(p) }

// Resize updates the pty window size.
func (b *Bridge) Resize(rows, cols uint16) error {
	return pty.Setsize(b.f, &pty.Winsize{Rows: rows, Cols: cols})
}

// SelectWindow switches which tmux window this attached client shows.
func (b *Bridge) SelectWindow(windowID string) error {
	return exec.Command("tmux", "select-window", "-t", windowID).Run()
}

// Close detaches: closes the pty and reaps the tmux attach client process.
func (b *Bridge) Close() error {
	if b.f != nil {
		_ = b.f.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait()
	}
	return nil
}
