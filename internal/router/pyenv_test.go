package router

import (
	"strings"
	"testing"
)

func envValue(t *testing.T, env []string, key string) string {
	t.Helper()
	val := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			val = v // last assignment wins, as os/exec's dedup does
		}
	}
	return val
}

// TestPythonEnvForcesUTF8 guards the fix for litellm dying at startup on
// Windows: its banner is non-ASCII and a cp1252 stdout makes printing it raise
// UnicodeEncodeError inside the FastAPI lifespan handler.
func TestPythonEnvForcesUTF8(t *testing.T) {
	env := pythonEnv()
	if got := envValue(t, env, "PYTHONIOENCODING"); got != "utf-8" {
		t.Errorf("PYTHONIOENCODING = %q, want utf-8", got)
	}
	if got := envValue(t, env, "PYTHONUTF8"); got != "1" {
		t.Errorf("PYTHONUTF8 = %q, want 1", got)
	}
}

func TestPythonEnvKeepsExtraAndParent(t *testing.T) {
	t.Setenv("COMMANDER_PYENV_PROBE", "parent")
	env := pythonEnv("ZEN_KEY=dummy")
	if got := envValue(t, env, "ZEN_KEY"); got != "dummy" {
		t.Errorf("extra env dropped: ZEN_KEY = %q", got)
	}
	if got := envValue(t, env, "COMMANDER_PYENV_PROBE"); got != "parent" {
		t.Errorf("parent env dropped: COMMANDER_PYENV_PROBE = %q", got)
	}
}

// A deliberate override — from the parent environment or from the caller —
// must survive, so anyone debugging an encoding issue can still pin their own.
func TestPythonEnvRespectsExplicitOverride(t *testing.T) {
	t.Setenv("PYTHONIOENCODING", "latin-1")
	if got := envValue(t, pythonEnv(), "PYTHONIOENCODING"); got != "latin-1" {
		t.Errorf("parent override lost: PYTHONIOENCODING = %q, want latin-1", got)
	}
	if got := envValue(t, pythonEnv("PYTHONUTF8=0"), "PYTHONUTF8"); got != "0" {
		t.Errorf("caller override lost: PYTHONUTF8 = %q, want 0", got)
	}
}
