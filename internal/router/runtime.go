package router

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

// The managed runtime is a self-contained Python venv, built on first run into
// the user config dir, holding litellm[proxy]. It exists so a DMG recipient who
// never `pip install`ed litellm can get a working routed-model runtime from an
// in-app button instead of a shell. LitellmBin probes ManagedLitellm (see the
// per-OS platformLitellmCandidates), so once InstallRuntime succeeds the proxy
// resolves it with no further wiring.

// ManagedRuntimeDir is the app-owned location for the first-run LiteLLM runtime.
// Under the user config dir (macOS: ~/Library/Application Support; Linux:
// ~/.config; Windows: %AppData%) so it is per-user and survives app reinstalls.
func ManagedRuntimeDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "Commander", "runtime")
}

// ManagedVenvDir is the venv built inside the managed runtime.
func ManagedVenvDir() string { return filepath.Join(ManagedRuntimeDir(), "venv") }

// runtimePins are the exact packages the first-run install pins. A floating
// `pip install litellm[proxy]` is NOT reproducible: litellm's proxy imports
// FastAPI internals (e.g. get_flat_dependant) that newer FastAPI removed, so a
// fresh resolve pulls an incompatible FastAPI and the proxy fails to import
// (ImportError -> the CLI's misleading "No module named 'proxy_server'"). Pin a
// tested litellm+FastAPI pair; bump both together only after re-verifying the
// proxy boots (Controller.Start -> /health/liveliness). See runtime_test.go.
var runtimePins = []string{
	"litellm[proxy]==1.83.9",
	"fastapi==0.124.4",
}

// ManagedLitellm and venvPython (venv exe layout) are per-OS: Unix uses
// bin/litellm, Windows Scripts\litellm.exe.

// RuntimeStatus tells the UI whether LiteLLM is usable and, if not, whether a
// first-run install can build it here.
type RuntimeStatus struct {
	Installed  bool   `json:"installed"`  // litellm resolvable now (managed OR a system/pip install)
	Path       string `json:"path"`       // the resolved litellm path when Installed
	Managed    bool   `json:"managed"`    // the app-managed venv is present
	Python     string `json:"python"`     // python3 discovered for building (empty => none)
	CanInstall bool   `json:"canInstall"` // a python3 is present, so a first-run install is possible
}

// Status snapshots the LiteLLM runtime situation for the frontend.
func Status() RuntimeStatus {
	st := RuntimeStatus{}
	if p, err := LitellmBin(); err == nil {
		st.Installed, st.Path = true, p
	}
	st.Managed = isExecFile(ManagedLitellm())
	st.Python = FindPython()
	st.CanInstall = st.Python != ""
	return st
}

// litellm[proxy] runs on a narrow Python window and fails outside it:
//   - below minPythonMinor (3.9, the macOS Command Line Tools interpreter): PEP
//     604 unions (`dict | None`) in eagerly-imported proxy modules fail to parse.
//   - above maxPythonMinor: not yet supported by litellm — e.g. on 3.14 the proxy
//     CLI's `from proxy_server import ...` raises ModuleNotFoundError and the
//     proxy never boots.
// So a first-run install must build against a Python inside [min, max], not just
// "the newest one found". Bump maxPythonMinor as litellm validates new releases.
const (
	minPythonMinor = 10
	maxPythonMinor = 13
)

// FindPython locates a python inside the litellm-supported version window to
// build the venv with. GUI apps launched from Finder inherit a bare PATH, so —
// as with litellm — we probe beyond PATH: explicit versioned interpreters
// (pythonX.Y) newest-first, then generic names, all gated to the window.
// COMMANDER_PYTHON overrides and skips the gate (the user vouches for it).
func FindPython() string {
	if p := os.Getenv("COMMANDER_PYTHON"); p != "" {
		if isExecFile(p) {
			return p
		}
		return ""
	}
	var cands []string
	// Explicit supported versions, newest-first, so we prefer a known-good one
	// over whatever generic `python3` happens to point at (often too new).
	for minor := maxPythonMinor; minor >= minPythonMinor; minor-- {
		name := fmt.Sprintf("python3.%d", minor)
		if p, err := exec.LookPath(name); err == nil {
			cands = append(cands, p)
		}
		cands = append(cands, platformVersionedPython(name)...)
	}
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			cands = append(cands, p)
		}
	}
	cands = append(cands, platformPythonCandidates()...)
	for _, c := range cands {
		if pythonUsable(c) {
			return c
		}
	}
	return ""
}

// pythonUsable reports whether c is an executable Python 3 inside the supported
// window. It asks the interpreter itself (a version baked into a path name can
// lie), tolerating a missing/incompatible interpreter by returning false.
func pythonUsable(c string) bool {
	out, err := exec.Command(c, "-c", "import sys;print(sys.version_info[0],sys.version_info[1])").Output()
	if err != nil {
		return false
	}
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &major, &minor); err != nil {
		return false
	}
	return major == 3 && minor >= minPythonMinor && minor <= maxPythonMinor
}

// InstallRuntime builds the managed LiteLLM runtime: a fresh venv at
// ManagedVenvDir with litellm[proxy] pip-installed. Each line of venv/pip output
// is handed to onLine for live progress. It removes any half-built venv first so
// a retried install starts clean. ctx cancellation aborts the in-flight step.
func InstallRuntime(ctx context.Context, python string, onLine func(string)) error {
	if python == "" {
		python = FindPython()
	}
	if python == "" {
		return fmt.Errorf("no Python 3.%d–3.%d found to build the runtime; install one (macOS: `brew install python@3.12` or python.org) and retry — the system Python 3.9 is too old for litellm and 3.14+ is not yet supported", minPythonMinor, maxPythonMinor)
	}
	venv := ManagedVenvDir()
	if err := os.MkdirAll(filepath.Dir(venv), 0o755); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	_ = os.RemoveAll(venv) // start clean so a retry never inherits a broken venv
	steps := [][]string{
		{python, "-m", "venv", venv},
		{venvPython(venv), "-m", "pip", "install", "--upgrade", "pip"},
		append([]string{venvPython(venv), "-m", "pip", "install"}, runtimePins...),
	}
	for _, args := range steps {
		if err := runStreaming(ctx, args, onLine); err != nil {
			return err
		}
	}
	if !isExecFile(ManagedLitellm()) {
		return fmt.Errorf("install finished but litellm is missing at %s", ManagedLitellm())
	}
	return nil
}

// runStreaming runs a command with stdout+stderr merged into a single stream,
// emitting each line via onLine. The command line itself is echoed first so the
// live log reads like a terminal.
func runStreaming(ctx context.Context, args []string, onLine func(string)) error {
	emit := func(s string) {
		if onLine != nil {
			onLine(s)
		}
	}
	emit("$ " + strings.Join(args, " "))
	cmd := proc.Hide(exec.CommandContext(ctx, args[0], args[1:]...))
	cmd.Env = pythonEnv()
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Start(); err != nil {
		w.Close()
		r.Close()
		return fmt.Errorf("start %s: %w", filepath.Base(args[0]), err)
	}
	w.Close() // drop the parent's write end so the scanner sees EOF when the child exits
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		emit(sc.Text())
	}
	r.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %w", filepath.Base(args[0]), err)
	}
	return nil
}
