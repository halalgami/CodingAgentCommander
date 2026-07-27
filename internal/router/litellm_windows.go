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
	var c []string
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
