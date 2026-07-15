package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ensureLoginPATH widens PATH when the app was launched from Finder/Dock.
// GUI-launched processes inherit launchd's bare "/usr/bin:/bin:/usr/sbin:/sbin",
// which hides tmux (homebrew), claude (~/.local/bin) and litellm — the app
// then sees an empty tmux world and every launch fails. Terminal launches
// (`make dev`, `open` from a shell) inherit the full PATH, which is why the
// bug only bit the installed .app. Asking the login shell once fixes every
// exec.Command in the process AND the env of the tmux server we spawn.
func ensureLoginPATH() {
	// launchd also omits LANG/LC_*: a C-locale tmux client then replaces every
	// non-ASCII AND control character in its output with "_" — including the
	// tab separators List parses by — so a live session reads as empty. The
	// same C locale would garble claude's TUI inside new panes.
	if os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" && os.Getenv("LC_CTYPE") == "" {
		os.Setenv("LANG", "en_US.UTF-8")
	}
	// No TERM → `tmux attach` refuses ("terminal does not support clear") and
	// the embedded terminal stays blank.
	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}

	if _, err := exec.LookPath("tmux"); err == nil {
		return // PATH already usable
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lc", "printf %s \"$PATH\"").Output()
	if err == nil {
		if p := strings.TrimSpace(string(out)); strings.Contains(p, "/") {
			os.Setenv("PATH", p)
		}
	}
	if _, err := exec.LookPath("tmux"); err == nil {
		return
	}

	// Login shell didn't help (or timed out): append the usual suspects.
	home, _ := os.UserHomeDir()
	extra := []string{
		"/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, ".local", "bin"),
	}
	os.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+strings.Join(extra, string(os.PathListSeparator)))
}
