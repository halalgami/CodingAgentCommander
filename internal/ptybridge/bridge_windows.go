//go:build windows

package ptybridge

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/UserExistsError/conpty"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
)

// Bridge is a Windows pseudo-console (ConPTY) running `tmux attach` against the
// psmux server. It is an io.ReadWriteCloser: Read yields the session's screen
// bytes, Write sends keystrokes. It mirrors the Unix (creack/pty) Bridge so the
// wsterm streamer is platform-agnostic. creack/pty has no working Windows
// backend (StartWithSize returns ErrUnsupported), so ConPTY is used directly.
type Bridge struct {
	session string
	cpty    *conpty.ConPty
}

// Attach opens a ConPTY running `tmux attach-session` for session, sized
// rows x cols, with the status bar hidden for a clean embed. psmux installs a
// `tmux` shim, so this uses the same command surface as the macOS build.
func Attach(session string, rows, cols uint16) (*Bridge, error) {
	// Hide chrome; ignore error if the session has none yet. proc.Hide keeps the
	// psmux console from flashing a window (Commander is a GUI app).
	_ = proc.Hide(exec.Command("tmux", "set-option", "-t", session, "status", "off")).Run()

	// Resolve the shim to an absolute path so ConPTY's CreateProcess doesn't
	// depend on how it parses a bare command name.
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux (psmux) not found on PATH: %w", err)
	}
	cmdline := quoteArg(tmuxPath) + " attach-session -t " + quoteArg(session)

	// xterm.js is xterm-256color compatible; hand psmux a TERM in case it reads
	// one, matching the macOS bridge.
	env := os.Environ()
	if os.Getenv("TERM") == "" {
		env = append(env, "TERM=xterm-256color")
	}

	// ConPTY dimensions are (width=cols, height=rows).
	cpty, err := conpty.Start(cmdline,
		conpty.ConPtyDimensions(int(cols), int(rows)),
		conpty.ConPtyEnv(env),
	)
	if err != nil {
		return nil, fmt.Errorf("conpty attach %s: %w", session, err)
	}
	return &Bridge{session: session, cpty: cpty}, nil
}

// Read implements io.Reader (screen bytes from the session).
func (b *Bridge) Read(p []byte) (int, error) { return b.cpty.Read(p) }

// Write implements io.Writer (keystrokes into the active pane).
func (b *Bridge) Write(p []byte) (int, error) { return b.cpty.Write(p) }

// Resize updates the pseudo-console size. ConPTY takes (width=cols, height=rows).
func (b *Bridge) Resize(rows, cols uint16) error {
	return b.cpty.Resize(int(cols), int(rows))
}

// SelectWindow switches which tmux window this attached client shows.
//
// The id goes through tmux.WindowTarget first: psmux strips the "@" off a window
// id and selects that *index* instead, so clicking a session card showed a
// different session's window (or, when no window sat at that index, silently
// nothing).
func (b *Bridge) SelectWindow(windowID string) error {
	target := tmux.WindowTarget(b.session, windowID)
	if target == "" {
		return fmt.Errorf("select-window: no window %s in session %s", windowID, b.session)
	}
	return proc.Hide(exec.Command("tmux", "select-window", "-t", target)).Run()
}

// Close detaches: closing the ConPTY terminates the attach client process.
func (b *Bridge) Close() error {
	if b.cpty != nil {
		return b.cpty.Close()
	}
	return nil
}

// quoteArg wraps s in double quotes when it contains whitespace, escaping any
// embedded quotes, so a shim path or session name with spaces survives the
// single command-line string CreateProcess parses.
func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
