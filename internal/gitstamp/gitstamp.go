// Package gitstamp tells whether a working tree's HEAD may have moved without
// running git: git appends to the HEAD reflog on every commit, checkout,
// reset, merge and rebase, so a stat of that file is a free change signal.
package gitstamp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stamp returns a fingerprint of root's HEAD reflog that changes whenever HEAD
// moves. ok is false when there is no reflog to read (not a repo, reflogs
// disabled, reftable storage); callers then fall back to asking git.
func Stamp(root string) (string, bool) {
	fi, ok := reflog(root)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano()), true
}

// Moved returns when root's HEAD last moved, per the reflog.
func Moved(root string) (time.Time, bool) {
	fi, ok := reflog(root)
	if !ok {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}

func reflog(root string) (os.FileInfo, bool) {
	gd, ok := gitDir(root)
	if !ok {
		return nil, false
	}
	fi, err := os.Stat(filepath.Join(gd, "logs", "HEAD"))
	if err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	return fi, true
}

// gitDir resolves root/.git: the directory itself, or the "gitdir: <path>" a
// linked worktree or submodule's .git file points to (each has its own HEAD
// reflog there).
func gitDir(root string) (string, bool) {
	p := filepath.Join(root, ".git")
	fi, err := os.Stat(p)
	if err != nil {
		return "", false
	}
	if fi.IsDir() {
		return p, true
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(b), "\n")
	d, found := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !found {
		return "", false
	}
	d = filepath.FromSlash(strings.TrimSpace(d))
	if !filepath.IsAbs(d) {
		d = filepath.Join(root, d)
	}
	return d, true
}
