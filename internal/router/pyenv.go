package router

import (
	"os"
	"strings"
)

// pythonEnv returns the parent environment plus extra, with UTF-8 stdio forced
// for the Python children this package spawns (litellm, venv, pip).
//
// Python takes its stdout/stderr encoding from the OS locale, and on Windows
// that is a legacy ANSI codepage — cp1252 on an en-US box. litellm prints a
// banner with box-drawing characters from inside its own startup event, and pip
// prints check marks; writing either to a cp1252 stream raises
// UnicodeEncodeError. For litellm that is fatal rather than cosmetic: the
// exception escapes the FastAPI lifespan handler, so the proxy logs
// "Application startup failed. Exiting." and never serves a request.
//
// PYTHONIOENCODING pins the stdio codec; PYTHONUTF8 puts the interpreter in
// UTF-8 mode so filesystem and locale conversions agree with it. Both are
// already the default on macOS and most Linux, so this stays one cross-platform
// path. A value inherited from the environment or passed in extra wins, leaving
// a deliberate override intact.
func pythonEnv(extra ...string) []string {
	env := append(os.Environ(), extra...)
	for _, kv := range []string{"PYTHONIOENCODING=utf-8", "PYTHONUTF8=1"} {
		key, _, _ := strings.Cut(kv, "=")
		if !hasEnvKey(env, key) {
			env = append(env, kv)
		}
	}
	return env
}

// hasEnvKey reports whether env assigns key a non-empty value. The comparison
// is case-insensitive because Windows environment names are. An empty
// assignment counts as absent: Python itself treats PYTHONIOENCODING="" as
// unset, so honouring it as an override would silently reinstate the cp1252
// default this whole file exists to avoid.
func hasEnvKey(env []string, key string) bool {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) && v != "" {
			return true
		}
	}
	return false
}
