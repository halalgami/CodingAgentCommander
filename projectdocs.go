package main

// Doc viewer, enumeration half: find the documents in one project.
//
// This walks a directory the user chose, so it is bounded on every axis —
// count, wall clock, depth — and it never follows a symlink: a loop hangs the
// walk, and a link out of the project silently widens the root.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

const (
	// docsMaxEntries bounds a listing of a directory nobody here chose.
	docsMaxEntries = 2000
	// docsMaxDepth bounds the fallback walk. Documents live near the top.
	docsMaxDepth = 12
)

// docsListBudget bounds enumeration: a network mount can make a walk of an
// ordinary tree take an unbounded amount of time. A var, not a const, so a
// test can shrink it — a budget nothing exercises is a budget nobody knows
// works.
var docsListBudget = 3 * time.Second

// docListExt is what the PALETTE lists: the shapes a document takes in a
// project Claude Code has been working in. It is deliberately narrower than
// what the reader accepts (spec R2.3) — listing every source file would flood
// a twelve-row palette, while a source file is still readable by path.
var docListExt = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".json": true,
	".yaml": true, ".yml": true, ".toml": true, ".csv": true, ".log": true,
}

// docsSkipDirs are skipped by the fallback walk. The git path needs no such
// list — .gitignore already covers these and everything else the project
// considers noise.
var docsSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
}

// DocEntry is one document in a listing. Rel uses forward slashes on every
// platform: the frontend joins and compares these as strings.
type DocEntry struct {
	Rel     string `json:"rel"`
	ModTime int64  `json:"modTime"` // unix seconds
	Size    int64  `json:"size"`
	// SinceStart is true when this document changed at or after the launch of
	// the session it was listed for. It is NOT authorship — this app does not
	// detect which files a session wrote (spec R3.3) — and it is always false
	// for a project-wide listing, which has no session to be relative to.
	SinceStart bool `json:"sinceStart"`
}

// DocListing is what the palette renders. Truncated is surfaced in the UI
// rather than hidden — a silent cap reads as "that is all the docs".
//
// Root is the RESOLVED root the listing was built from (docRoot's return
// value, absolute and symlink-free) — not merely an echo of whatever string
// was passed in. The frontend can have a stale or empty root on hand (a
// session card whose stats have not polled a cwd yet, for instance); Root is
// the value Go itself already validated, so the frontend and the read/open
// guard can no longer disagree about which folder a row belongs to.
type DocListing struct {
	Entries   []DocEntry `json:"entries"`
	Root      string     `json:"root"`
	Truncated bool       `json:"truncated"`
}

// docRoot resolves and validates a project root, returning its real
// (symlink-free) absolute path. An empty, missing or non-directory root is an
// error rather than an empty listing, so a mistyped root is distinguishable
// from a project with no documents.
func docRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("no project folder given")
	}
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("%s could not be resolved: %w", root, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s could not be opened: %w", root, err)
	}
	fi, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("%s could not be opened: %w", root, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s is not a folder", root)
	}
	return real, nil
}

// gitListDocs enumerates with one `git ls-files` call: tracked PLUS
// untracked-but-not-ignored.
//
// Nothing stats root/.git first. git resolves the repository itself and,
// because cmd.Dir is root, lists only what is under it with paths relative to
// it — so a monorepo package opened as a project root still gets
// gitignore-aware listing instead of falling back to the walk.
//
// Two flags are load-bearing. -z, because git QUOTES any path containing a
// non-ASCII or unusual byte ("\303\251.md") and the quoted name does not
// exist. :(icase), because git pathspecs are case-sensitive while the walk
// lowercases the extension.
func gitListDocs(root string) ([]string, bool, error) {
	args := []string{"ls-files", "-co", "--exclude-standard", "-z", "--"}
	for ext := range docListExt {
		args = append(args, ":(icase)*"+ext)
	}
	sort.Strings(args[5:]) // stable argv, so a failure is reproducible
	ctx, cancel := context.WithTimeout(context.Background(), docsListBudget)
	defer cancel()
	cmd := proc.Hide(exec.CommandContext(ctx, "git", args...))
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// A budget overrun is TRUNCATION, not failure: falling back would burn
		// the same budget again on the same slow tree and answer worse.
		if ctx.Err() == context.DeadlineExceeded {
			names, _ := parseLsFiles(out)
			return names, true, nil
		}
		return nil, false, fmt.Errorf("git ls-files: %w", err)
	}
	names, truncated := parseLsFiles(out)
	return names, truncated, nil
}

// parseLsFiles splits a -z payload, deduplicates and caps it.
//
// Dedupe is not defensive: `git ls-files -c` prints one line PER STAGE, so
// during a merge conflict — an ordinary state in a project being worked in —
// the same file arrives three times and the palette shows three identical rows.
func parseLsFiles(out []byte) ([]string, bool) {
	seen := map[string]bool{}
	var names []string
	for _, n := range strings.Split(string(out), "\x00") {
		if n == "" || seen[n] {
			continue
		}
		if len(names) >= docsMaxEntries {
			return names, true // something is left over: that is truncation
		}
		seen[n] = true
		names = append(names, n)
	}
	return names, false
}

// walkListDocs is the fallback for a project that is not a repository, which
// is ordinary rather than exceptional.
func walkListDocs(root string) ([]string, bool) {
	var names []string
	truncated := false
	start := time.Now()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable subtree is skipped, not fatal
		}
		if time.Since(start) > docsListBudget {
			truncated = true
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if docsSkipDirs[d.Name()] || strings.Count(rel, string(filepath.Separator))+1 > docsMaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		if !docListExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		// Checked BEFORE the append, so exactly the cap is not truncation: this
		// only fires when an ADDITIONAL match exists beyond the cap (mirrors
		// parseLsFiles's check-before-add).
		if len(names) >= docsMaxEntries {
			truncated = true
			return filepath.SkipAll
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	return names, truncated
}

// statDocs turns names into entries, dropping anything that is not a plain
// readable file. git reports names, not kinds, so this is also where a
// symlinked document coming out of `ls-files` is refused.
func statDocs(root string, names []string) []DocEntry {
	entries := make([]DocEntry, 0, len(names))
	for _, n := range names {
		rel := filepath.ToSlash(n)
		if !docListExt[strings.ToLower(filepath.Ext(rel))] {
			continue
		}
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		entries = append(entries, DocEntry{
			Rel: rel, ModTime: fi.ModTime().Unix(), Size: fi.Size(),
		})
	}
	return entries
}
