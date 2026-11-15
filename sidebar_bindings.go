package main

import (
	"os"
	"path/filepath"

	"github.com/halalgami/CodingAgentCommander/internal/sidebar"
)

// sidebarPath sits beside windows.json: both are keyed by tmux window id and
// pruned on the same reconcile pass.
func (a *App) sidebarPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "commander", "sidebar.json")
}

// SidebarLayout returns the sidebar's groups and per-section session order,
// already merged with the live windows by the last ListSessions reconcile.
func (a *App) SidebarLayout() sidebar.Layout { return a.sidebar.Get() }

// SetSidebarLayout stores a layout the frontend edited (normalized first).
func (a *App) SetSidebarLayout(l sidebar.Layout) error { return a.sidebar.Set(l) }
