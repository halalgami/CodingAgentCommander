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
// not be on a GUI app's PATH.
func platformLitellmCandidates(home string) []string {
	var c []string
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
