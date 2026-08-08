package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the whole package test binary from the real user config.
// Several tests drive LaunchSession / EnableRemoteControl / SwapModel with a
// fake host, and LaunchSession records project-open history beside
// configPath() (projects.json). Without an isolated default, those tests would
// write — and could clobber — the real ~/.config/commander/{projects.json,
// config.toml}. Pointing COMMANDER_CONFIG at a throwaway temp dir for the test
// run keeps every test off the real config. Tests that need their own config
// still call t.Setenv("COMMANDER_CONFIG", ...), which overrides this default.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "commander-test-config")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
