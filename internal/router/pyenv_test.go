package router

import (
	"os"
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

// clearEnv unsets key for the duration of the test. t.Setenv has no unset
// counterpart, but it registers the restore we need, so set-then-unset leaves
// the original value reinstated at cleanup.
func clearEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "placeholder")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

// TestPythonEnvForcesUTF8 guards the fix for litellm dying at startup on
// Windows: its banner is non-ASCII and a cp1252 stdout makes printing it raise
// UnicodeEncodeError inside the FastAPI lifespan handler.
//
// The vars are cleared first because a developer machine may already export
// them — this asserts what pythonEnv supplies, not what the box happens to have.
func TestPythonEnvForcesUTF8(t *testing.T) {
	clearEnv(t, "PYTHONIOENCODING")
	clearEnv(t, "PYTHONUTF8")

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
// utf-8:surrogateescape is a real value seen in the wild and must not be
// flattened to plain utf-8.
func TestPythonEnvRespectsExplicitOverride(t *testing.T) {
	t.Setenv("PYTHONIOENCODING", "utf-8:surrogateescape")
	if got := envValue(t, pythonEnv(), "PYTHONIOENCODING"); got != "utf-8:surrogateescape" {
		t.Errorf("parent override lost: PYTHONIOENCODING = %q", got)
	}
	if got := envValue(t, pythonEnv("PYTHONUTF8=0"), "PYTHONUTF8"); got != "0" {
		t.Errorf("caller override lost: PYTHONUTF8 = %q, want 0", got)
	}
}

// An exported-but-empty value is not an override: Python treats it as unset, so
// deferring to it would hand litellm the cp1252 default and crash it.
func TestPythonEnvIgnoresEmptyOverride(t *testing.T) {
	t.Setenv("PYTHONIOENCODING", "")
	if got := envValue(t, pythonEnv(), "PYTHONIOENCODING"); got != "utf-8" {
		t.Errorf("empty override should not win: PYTHONIOENCODING = %q, want utf-8", got)
	}
}
