// Package tmux hosts Claude Code sessions as tmux windows.
package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// tmuxCmd builds an `tmux` command with a console window suppressed on Windows
// (proc.Hide is a no-op elsewhere), so the frequent poll/reconcile calls don't
// flash a console. Use this instead of tmuxCmd(…) throughout.
func tmuxCmd(args ...string) *exec.Cmd { return proc.Hide(exec.Command("tmux", args...)) }

// LaunchSpec describes a session to create.
type LaunchSpec struct {
	SessionName string
	WindowName  string
	Dir         string
	Env         map[string]string
	// ClearEnv names every variable Commander may have injected on some
	// earlier launch — not just the ones this launch sets. Launch removes all
	// of them before applying Env, so a routed session's proxy variables can
	// never be inherited by a native one. See Launch for why omitting a
	// variable is not the same as clearing it.
	ClearEnv []string
	Command  []string
}

// WindowState is a live tmux window. What Commander knows about it beyond this
// — the model, the Remote Control flag — is in internal/winstate, not in tmux
// options: psmux does not scope those per window. See that package.
type WindowState struct {
	ID     string
	Name   string
	Active bool
	Cwd    string // pane_current_path — used to reconcile surviving windows
}

// Host manages Claude Code sessions.
type Host interface {
	Launch(spec LaunchSpec) (WindowState, error)
	List(session string) ([]WindowState, error)
	Kill(session, windowID string) error
	Rename(session, windowID, name string) error
	SendKeys(windowID, text string) error
}

// ExecHost implements Host by shelling out to the real tmux binary.
type ExecHost struct {
	// launchMu serialises Launch. Env is staged in the session environment for
	// the moment it takes to create the window (see Launch), so two concurrent
	// launches could otherwise hand one window the other's model.
	launchMu sync.Mutex
}

// NewExecHost returns an ExecHost.
func NewExecHost() *ExecHost { return &ExecHost{} }

func (h *ExecHost) hasSession(name string) bool {
	return tmuxCmd("has-session", "-t", name).Run() == nil
}

// Launch creates the session (if absent) or a new window (if present),
// applying Dir and Env, and returns the new window's id.
//
// Env is staged in the *session* environment for the moment it takes to spawn
// the window, and cleared again immediately after, rather than relying on `-e`
// alone. On real tmux `-e` is per-window and would be enough. psmux — the shim
// this drives on Windows — behaves differently in two ways that together sent
// every session to whichever model happened to be launched first:
//
//   - `-e` on new-window is ignored outright, so a window never got its own
//     model at all;
//   - `-e` on the new-session that starts the server lands in the session and
//     server-wide environments, which every window created later inherits.
//
// So the first launch of the day fixed the environment for all of them: a
// routed model left ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN behind, and
// every later "native Anthropic" window silently went to the LiteLLM proxy.
// Staging gives psmux the per-window value it otherwise drops; clearing stops
// it reaching the next window. The window's process has its own copy of the
// environment by the time we clear, so it is unaffected.
func (h *ExecHost) Launch(spec LaunchSpec) (WindowState, error) {
	h.launchMu.Lock()
	defer h.launchMu.Unlock()

	// Everything to clear: what Commander manages, plus whatever this launch
	// sets, in case the two ever drift apart.
	stale := append(append([]string{}, spec.ClearEnv...), envKeys(spec.Env)...)

	var args []string
	if h.hasSession(spec.SessionName) {
		// Clear first: the session may still carry values from a build that
		// predates this fix, or from the server-wide environment.
		h.clearEnv(spec.SessionName, stale)
		for k, v := range spec.Env {
			_ = tmuxCmd("set-environment", "-t", spec.SessionName, k, v).Run()
		}
		args = []string{"new-window", "-t", spec.SessionName, "-n", spec.WindowName}
	} else {
		// No session to stage into yet, and psmux does honour `-e` here. Clear
		// the server-wide environment first, or new-session copies it in.
		h.clearEnv("", stale)
		args = []string{"new-session", "-d", "-s", spec.SessionName, "-n", spec.WindowName}
	}
	args = append(args, "-c", spec.Dir, "-P", "-F", "#{window_id}")
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, spec.Command...)

	out, err := tmuxCmd(args...).Output()
	// Unstage on the way out, success or not: a failed launch that left values
	// behind would hand them to the next window.
	h.clearEnv(spec.SessionName, stale)
	if err != nil {
		return WindowState{}, fmt.Errorf("tmux launch: %w", err)
	}
	id := strings.TrimSpace(string(out))
	return WindowState{ID: id, Name: spec.WindowName, Active: true}, nil
}

// envKeys returns the names in env, in no particular order.
func envKeys(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k := range env {
		out = append(out, k)
	}
	return out
}

// clearEnv removes names from a session's environment and from the server-wide
// (global) one that psmux seeds from new-session's -e flags and that every
// later session copies. An empty session clears the global scope only. Unsetting
// a name that was never set is not an error, so this is safe to call blind.
//
// Only names Commander itself injects are touched — but note that includes the
// case where the user exported one into the tmux server themselves: inside
// Commander's session the injected value has to win, or the model shown on the
// card is not the model answering.
func (h *ExecHost) clearEnv(session string, names []string) {
	for _, n := range names {
		if session != "" {
			_ = tmuxCmd("set-environment", "-t", session, "-u", n).Run()
		}
		_ = tmuxCmd("set-environment", "-g", "-u", n).Run()
	}
}

// List returns the windows of a session. A genuinely absent session (or no
// tmux server yet) is empty-not-error; anything else — including a missing
// tmux binary — must surface, or the UI silently blanks (the Finder-launch
// bare-PATH failure mode).
func (h *ExecHost) List(session string) ([]WindowState, error) {
	out, err := tmuxCmd("list-windows", "-t", session,
		"-F", "#{window_id}\t#{window_name}\t#{window_active}\t#{pane_current_path}").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := string(ee.Stderr)
			if strings.Contains(msg, "can't find session") ||
				strings.Contains(msg, "no server running") ||
				strings.Contains(msg, "error connecting") {
				return nil, nil
			}
			return nil, fmt.Errorf("tmux list-windows: %w: %s", err, strings.TrimSpace(msg))
		}
		return nil, fmt.Errorf("tmux list-windows: %w", err)
	}
	var ws []WindowState
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 4)
		if len(f) < 3 {
			continue
		}
		w := WindowState{ID: f[0], Name: f[1], Active: f[2] == "1"}
		if len(f) >= 4 {
			w.Cwd = f[3]
		}
		ws = append(ws, w)
	}
	return ws, nil
}

// WindowTarget resolves a window id (@5) to a "session:index" target string.
//
// Window ids are the natural target on upstream tmux — they are stable while
// indexes shift — but psmux, the Windows shim, does not resolve them, and does
// so silently and inconsistently. Measured on psmux 3.3.7:
//
//   - select-window drops the "@" and treats the rest as a window *index*, so
//     `-t @2` selected whichever window sat at index 2;
//   - kill-window resolved the id against a *different session* that happened
//     to have a window with that id, killing the wrong session's window;
//   - rename-window silently did nothing;
//   - send-keys and capture-pane, alone, resolved ids correctly.
//
// Indexes are understood by both, so every command that targets a window goes
// through here. Indexes move when windows die, so this resolves immediately
// before use and never caches. An unknown id yields "" — treat that as a window
// that has already gone rather than falling back to the raw id, which is what
// made a mis-resolved target destructive in the first place.
func WindowTarget(session, windowID string) string {
	if session == "" || windowID == "" {
		return ""
	}
	out, err := tmuxCmd("list-windows", "-t", session,
		"-F", "#{window_id}\t#{session_name}:#{window_index}").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		id, target, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && id == windowID {
			return target
		}
	}
	return ""
}

// Kill removes a window by id.
func (h *ExecHost) Kill(session, windowID string) error {
	target := WindowTarget(session, windowID)
	if target == "" {
		return fmt.Errorf("tmux kill-window: no window %s in session %s", windowID, session)
	}
	if err := tmuxCmd("kill-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("tmux kill-window %s: %w", target, err)
	}
	return nil
}

// Rename changes a window's display name.
func (h *ExecHost) Rename(session, windowID, name string) error {
	target := WindowTarget(session, windowID)
	if target == "" {
		return fmt.Errorf("tmux rename-window: no window %s in session %s", windowID, session)
	}
	if err := tmuxCmd("rename-window", "-t", target, name).Run(); err != nil {
		return fmt.Errorf("tmux rename-window %s: %w", target, err)
	}
	return nil
}

// SendKeys types text into a window's active pane followed by Enter. -l sends
// the text literally (no key-name interpretation), so slash commands and
// spaces arrive verbatim at claude's input line.
func (h *ExecHost) SendKeys(windowID, text string) error {
	if err := tmuxCmd("send-keys", "-t", windowID, "-l", text).Run(); err != nil {
		return fmt.Errorf("send-keys: %w", err)
	}
	return tmuxCmd("send-keys", "-t", windowID, "Enter").Run()
}

