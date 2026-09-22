package delegate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the whole package test binary from the real user state
// directory. Run() writes a transcript on every call and StartProxy writes a
// generated config, both under StateDir() — so without an isolated default the
// stub-driven tests litter ~/.local/state/commander/delegate (measured: 280
// stray transcripts in one day, several sharing a millisecond, some empty).
//
// This mirrors the root package's TestMain, which pins COMMANDER_CONFIG for
// exactly the same reason after the LaunchSession tests were found writing into
// the real projects.json. Tests needing their own location still call
// t.Setenv(StateEnv, ...), which overrides this default.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "commander-delegate-test-state")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv(StateEnv, filepath.Join(dir, "state")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
