package router

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManagedPathsNested(t *testing.T) {
	// The venv lives under the runtime dir, and the litellm script under the venv.
	if !strings.HasPrefix(ManagedVenvDir(), ManagedRuntimeDir()) {
		t.Errorf("venv %q not under runtime %q", ManagedVenvDir(), ManagedRuntimeDir())
	}
	if !strings.HasPrefix(ManagedLitellm(), ManagedVenvDir()) {
		t.Errorf("litellm %q not under venv %q", ManagedLitellm(), ManagedVenvDir())
	}
	// venvPython points inside the given venv.
	if got := venvPython(ManagedVenvDir()); !strings.HasPrefix(got, ManagedVenvDir()) {
		t.Errorf("venvPython %q not under venv %q", got, ManagedVenvDir())
	}
	// Windows uses .exe console scripts; Unix does not.
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(ManagedLitellm(), ".exe") {
			t.Errorf("windows litellm should end .exe: %q", ManagedLitellm())
		}
	} else if strings.HasSuffix(ManagedLitellm(), ".exe") {
		t.Errorf("unix litellm should not end .exe: %q", ManagedLitellm())
	}
}

func TestManagedLitellmIsAProbeCandidate(t *testing.T) {
	// LitellmBin must probe the managed venv, so a first-run install is found
	// with no extra wiring. It should appear in the platform candidate list.
	home, _ := os.UserHomeDir()
	found := false
	for _, c := range platformLitellmCandidates(home) {
		if c == ManagedLitellm() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ManagedLitellm() %q not among platform candidates", ManagedLitellm())
	}
}

func TestFindPythonOverride(t *testing.T) {
	// A non-executable override resolves to empty (never a bad interpreter).
	t.Setenv("COMMANDER_PYTHON", filepath.Join(t.TempDir(), "does-not-exist"))
	if p := FindPython(); p != "" {
		t.Errorf("bad COMMANDER_PYTHON should yield empty, got %q", p)
	}

	// An executable override is honored verbatim.
	dir := t.TempDir()
	fake := filepath.Join(dir, "python3")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMANDER_PYTHON", fake)
	if runtime.GOOS != "windows" { // isExecFile needs the exec bit, which Windows ignores
		if p := FindPython(); p != fake {
			t.Errorf("COMMANDER_PYTHON not honored: got %q want %q", p, fake)
		}
	}
}

func TestStatusReflectsPython(t *testing.T) {
	st := Status()
	if (st.Python != "") != st.CanInstall {
		t.Errorf("CanInstall (%v) should track Python presence (%q)", st.CanInstall, st.Python)
	}
	if st.Installed && st.Path == "" {
		t.Error("Installed true but Path empty")
	}
}

func TestInstallRuntimeNoPython(t *testing.T) {
	// With no python discoverable and no override, InstallRuntime fails fast with
	// a guiding message rather than building a broken venv.
	t.Setenv("COMMANDER_PYTHON", filepath.Join(t.TempDir(), "nope"))
	err := InstallRuntime(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "Python 3") {
		t.Fatalf("want a 'no Python 3' error, got %v", err)
	}
}

func TestPythonWindowSane(t *testing.T) {
	// The supported window must be non-empty; a min > max would make FindPython
	// reject every interpreter and silently disable the first-run install.
	if minPythonMinor > maxPythonMinor {
		t.Fatalf("empty python window: min %d > max %d", minPythonMinor, maxPythonMinor)
	}
}

func TestRuntimePinsAreVersioned(t *testing.T) {
	// The install must pin exact versions — a floating litellm[proxy] pulls an
	// incompatible FastAPI and the proxy fails to boot. Guard against a stray
	// un-pin, and make sure the FastAPI pin (the actual break) stays present.
	if len(runtimePins) == 0 {
		t.Fatal("runtimePins empty — install would float and can break")
	}
	var litellm, fastapi bool
	for _, p := range runtimePins {
		if !strings.Contains(p, "==") {
			t.Errorf("pin %q is not exact-version (missing ==)", p)
		}
		if strings.HasPrefix(p, "litellm") {
			litellm = true
		}
		if strings.HasPrefix(p, "fastapi") {
			fastapi = true
		}
	}
	if !litellm || !fastapi {
		t.Errorf("pins must include both litellm and fastapi, got %v", runtimePins)
	}
}
