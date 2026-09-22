package delegate

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateDirHonoursTheEnvOverride(t *testing.T) {
	// Without an override, Run() writes a transcript per call into the REAL
	// ~/.local/state/commander/delegate — so the suite littered a user's home
	// with 280 files before this existed. app.go's configPath/COMMANDER_CONFIG
	// is the precedent being mirrored.
	want := filepath.Join(t.TempDir(), "somewhere")
	t.Setenv(StateEnv, want)
	if got := StateDir(); got != want {
		t.Errorf("StateDir() = %q, want the %s override %q", got, StateEnv, want)
	}
}

func TestStateDirDefaultIsAbsoluteAndUnderHome(t *testing.T) {
	t.Setenv(StateEnv, "")
	got := StateDir()
	if !filepath.IsAbs(got) {
		t.Errorf("StateDir() = %q, must be absolute: a relative path makes MkdirAll scribble into the worker's cwd", got)
	}
	if !strings.HasSuffix(got, filepath.Join("commander", "delegate")) {
		t.Errorf("StateDir() = %q, want it to end in commander/delegate", got)
	}
}
