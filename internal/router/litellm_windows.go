//go:build windows

package router

import (
	"os"
	"path/filepath"
)

// isExecFile reports whether p is an existing regular file. Windows has no
// execute permission bit — runnability is decided by extension (PATHEXT) at
// exec time — so any regular file the user points COMMANDER_LITELLM at, or a
// resolved Scripts\litellm.exe, is accepted; a non-runnable path fails later at
// Start rather than being silently rejected here.
func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// platformLitellmCandidates lists common pip "console script" locations on
// Windows. pip drops entry points into a Scripts directory that is often not on
// a GUI app's PATH (per-user python.org and pip --user installs especially).
func platformLitellmCandidates(home string) []string {
	c := []string{ManagedLitellm()} // app-managed venv (first-run install) wins once built
	for _, base := range []string{
		filepath.Join(os.Getenv("APPDATA"), "Python"),                  // pip --user
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python"), // python.org per-user
	} {
		if base == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(base, "*", "Scripts", "litellm.exe"))
		c = append(c, matches...)
	}
	c = append(c, filepath.Join(home, ".local", "bin", "litellm.exe"))
	return c
}

// ManagedLitellm is the litellm console script inside the managed venv.
func ManagedLitellm() string { return filepath.Join(ManagedVenvDir(), "Scripts", "litellm.exe") }

// venvPython is the interpreter inside a venv, used to drive pip during install.
func venvPython(venv string) string { return filepath.Join(venv, "Scripts", "python.exe") }

// platformPythonCandidates lists python.exe locations off a GUI app's bare PATH.
// Version-gated by the caller.
func platformPythonCandidates() []string {
	var c []string
	for _, base := range []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python"), // python.org per-user
	} {
		if base == "" {
			continue
		}
		// python.org lays out ...\Programs\Python\Python3XX\python.exe
		matches, _ := filepath.Glob(filepath.Join(base, "Python*", "python.exe"))
		c = append(c, matches...)
	}
	return c
}

// platformVersionedPython lists python.exe locations for a specific pythonX.Y.
// Windows has no pythonX.Y console name, so the python.org per-version layout
// (PythonXY\python.exe) is the closest thing; caller version-gates anyway.
func platformVersionedPython(name string) []string {
	// name is "python3.12" -> "312"
	digits := ""
	for _, r := range name {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	base := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python")
	if base == "" || digits == "" {
		return nil
	}
	return []string{filepath.Join(base, "Python"+digits, "python.exe")}
}
