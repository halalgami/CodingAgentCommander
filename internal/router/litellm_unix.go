//go:build !windows

package router

import (
	"os"
	"path/filepath"
)

// isExecFile reports whether p is a regular file with an execute bit set.
func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// platformLitellmCandidates lists common pip install locations on Unix that may
// not be on a GUI app's PATH. The app-managed venv (first-run install) is
// probed first so it wins once built.
func platformLitellmCandidates(home string) []string {
	c := []string{ManagedLitellm()}
	// pip --user on macOS: ~/Library/Python/<X.Y>/bin/litellm
	matches, _ := filepath.Glob(filepath.Join(home, "Library", "Python", "*", "bin", "litellm"))
	c = append(c, matches...)
	c = append(c,
		filepath.Join(home, ".local", "bin", "litellm"), // pip --user on Linux/mac
		"/opt/homebrew/bin/litellm",
		"/usr/local/bin/litellm",
	)
	return c
}

// ManagedLitellm is the litellm console script inside the managed venv.
func ManagedLitellm() string { return filepath.Join(ManagedVenvDir(), "bin", "litellm") }

// venvPython is the interpreter inside a venv, used to drive pip during install.
func venvPython(venv string) string { return filepath.Join(venv, "bin", "python") }

// platformPythonCandidates lists generic python3 locations off a GUI app's bare
// PATH. Version-gated by the caller.
func platformPythonCandidates() []string {
	return []string{
		"/opt/homebrew/bin/python3", // Apple-silicon Homebrew
		"/usr/local/bin/python3",    // Intel Homebrew / python.org
		"/usr/bin/python3",          // macOS Command Line Tools / Linux system
	}
}

// platformVersionedPython lists bin-dir locations for a specific pythonX.Y that
// a GUI app's PATH may miss (Homebrew keg bins especially).
func platformVersionedPython(name string) []string {
	return []string{
		filepath.Join("/opt/homebrew/bin", name), // Apple-silicon Homebrew
		filepath.Join("/usr/local/bin", name),    // Intel Homebrew / python.org
		// python.org framework builds: /Library/Frameworks/Python.framework/Versions/3.12/bin/python3.12
		filepath.Join("/Library/Frameworks/Python.framework/Versions", name[len("python"):], "bin", name),
	}
}
