package config

import (
	"os"
	"path/filepath"
)

// UserPath is the per-user config.toml location:
// ~/.config/commander/config.toml on every platform.
//
// Deliberately NOT os.UserConfigDir(), which on macOS resolves to
// ~/Library/Application Support — the app has always used ~/.config, and moving
// it would orphan every existing install's catalog.
//
// app.go holds a private copy of this (configPath). Phase 1 of delegation must
// not edit app.go, so these three lines are duplicated on purpose; collapse
// them when app.go is next touched for an unrelated reason.
func UserPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "commander", "config.toml")
	}
	return filepath.Join(home, ".config", "commander", "config.toml")
}
