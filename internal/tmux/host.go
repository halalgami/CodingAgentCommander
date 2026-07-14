// Package tmux hosts Claude Code sessions as tmux windows.
package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// modelOption is the tmux user option Commander stamps on each window so a
// surviving window's model survives a rename or app restart (the window name is
// user-editable and unreliable).
const modelOption = "@commander_model"

// LaunchSpec describes a session to create.
type LaunchSpec struct {
	SessionName string
	WindowName  string
	Dir         string
	Env         map[string]string
	Command     []string
	ModelID     string // stamped as @commander_model for durable reconciliation
}

// WindowState is a live tmux window.
type WindowState struct {
	ID      string
	Name    string
	Active  bool
	Cwd     string // pane_current_path — used to reconcile surviving windows
	ModelID string // @commander_model — durable model marker (may be empty)
}

// Host manages Claude Code sessions.
type Host interface {
	Launch(spec LaunchSpec) (WindowState, error)
	List(session string) ([]WindowState, error)
	Kill(session, windowID string) error
	Rename(windowID, name string) error
	SendKeys(windowID, text string) error
	SetOption(windowID, name, value string) error
	GetOption(windowID, name string) (string, error)
}

// ExecHost implements Host by shelling out to the real tmux binary.
type ExecHost struct{}

// NewExecHost returns an ExecHost.
func NewExecHost() *ExecHost { return &ExecHost{} }

func (h *ExecHost) hasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// Launch creates the session (if absent) or a new window (if present),
// applying Dir and Env, and returns the new window's id.
func (h *ExecHost) Launch(spec LaunchSpec) (WindowState, error) {
	var args []string
	if h.hasSession(spec.SessionName) {
		args = []string{"new-window", "-t", spec.SessionName, "-n", spec.WindowName}
	} else {
		args = []string{"new-session", "-d", "-s", spec.SessionName, "-n", spec.WindowName}
	}
	args = append(args, "-c", spec.Dir, "-P", "-F", "#{window_id}")
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, spec.Command...)

	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return WindowState{}, fmt.Errorf("tmux launch: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if spec.ModelID != "" {
		// Best-effort durable marker; a failure only degrades reconciliation.
		_ = exec.Command("tmux", "set-option", "-t", id, modelOption, spec.ModelID).Run()
	}
	return WindowState{ID: id, Name: spec.WindowName, Active: true, ModelID: spec.ModelID}, nil
}

// List returns the windows of a session.
func (h *ExecHost) List(session string) ([]WindowState, error) {
	if !h.hasSession(session) {
		return nil, nil
	}
	out, err := exec.Command("tmux", "list-windows", "-t", session,
		"-F", "#{window_id}\t#{window_name}\t#{window_active}\t#{pane_current_path}\t#{"+modelOption+"}").Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-windows: %w", err)
	}
	var ws []WindowState
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 5)
		if len(f) < 3 {
			continue
		}
		w := WindowState{ID: f[0], Name: f[1], Active: f[2] == "1"}
		if len(f) >= 4 {
			w.Cwd = f[3]
		}
		if len(f) == 5 {
			w.ModelID = f[4]
		}
		ws = append(ws, w)
	}
	return ws, nil
}

// Kill removes a window by id.
func (h *ExecHost) Kill(session, windowID string) error {
	if err := exec.Command("tmux", "kill-window", "-t", windowID).Run(); err != nil {
		return fmt.Errorf("tmux kill-window %s: %w", windowID, err)
	}
	return nil
}

// Rename changes a window's display name.
func (h *ExecHost) Rename(windowID, name string) error {
	if err := exec.Command("tmux", "rename-window", "-t", windowID, name).Run(); err != nil {
		return fmt.Errorf("tmux rename-window %s: %w", windowID, err)
	}
	return nil
}

// SendKeys types text into a window's active pane followed by Enter. -l sends
// the text literally (no key-name interpretation), so slash commands and
// spaces arrive verbatim at claude's input line.
func (h *ExecHost) SendKeys(windowID, text string) error {
	if err := exec.Command("tmux", "send-keys", "-t", windowID, "-l", text).Run(); err != nil {
		return fmt.Errorf("send-keys: %w", err)
	}
	return exec.Command("tmux", "send-keys", "-t", windowID, "Enter").Run()
}

// SetOption stamps a window-scoped tmux user option (durable across app restarts).
func (h *ExecHost) SetOption(windowID, name, value string) error {
	return exec.Command("tmux", "set-option", "-w", "-t", windowID, name, value).Run()
}

// GetOption reads a window-scoped option; "" means unset.
func (h *ExecHost) GetOption(windowID, name string) (string, error) {
	out, err := exec.Command("tmux", "show-options", "-w", "-v", "-t", windowID, name).Output()
	if err != nil {
		return "", nil // unset options can exit non-zero on some tmux versions; treat as unset
	}
	return strings.TrimSpace(string(out)), nil
}
