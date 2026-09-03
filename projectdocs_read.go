package main

// Doc viewer, reading half: turn a (root, rel) pair from the frontend into
// bytes, or refuse.
//
// Every check REFUSES rather than repairs. filepath.Base("../../x") is "x", so
// sanitising an attack turns it into an accepted file — the packgen import
// lesson, applied verbatim.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// docsMaxBytes is the read cap. Past it the viewer offers external open only,
// rather than pushing a giant string across the binding.
const docsMaxBytes = 2 << 20 // 2 MB

// docsSniffBytes is how much of a file decides whether it is text.
const docsSniffBytes = 8 << 10

// docPath re-derives and re-validates a path arriving from the frontend,
// returning its resolved real path.
//
// The security boundary is ONE check: containment of the fully resolved path
// inside the resolved root. The lexical pass before it is a cheap filter for
// the obvious cases that avoids touching the filesystem — useful, but not
// load-bearing, and a mutation proves it (deleting it changes no outcome).
//
// ".." is rejected as a SEGMENT, not as a prefix: "..hidden.md" is a legal
// filename that git will list, and refusing it would put an unopenable row in
// the palette.
//
// Known limit, stated rather than implied: a HARDLINK to a file outside the
// project is indistinguishable from an ordinary file and is readable. Making
// one needs local write access inside the project on the same filesystem, and
// git cannot ship one.
func docPath(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("no document chosen")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("refusing to open %s: it is not a path inside the project", rel)
	}
	// Backslash is a separator only where it is one. On Unix it is a legal
	// character in a filename, and refusing it there would reject a file this
	// app itself listed.
	scan := rel
	if runtime.GOOS == "windows" {
		scan = strings.ReplaceAll(rel, `\`, "/")
	}
	for _, seg := range strings.Split(scan, "/") {
		if seg == ".." {
			return "", fmt.Errorf("refusing to open %s: it is not a path inside the project", rel)
		}
	}
	realRoot, err := docRoot(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(filepath.Join(realRoot, filepath.FromSlash(rel)))
	// Lstat does NOT follow the final symlink, unlike Stat. That is what makes
	// a link pointing INSIDE the project a refusal rather than a read.
	fi, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("%s is no longer in the project: %w", rel, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to open %s: it is a symlink", rel)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to open %s: it is not a file", rel)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s is no longer in the project: %w", rel, err)
	}
	if err := docContained(realRoot, real); err != nil {
		return "", fmt.Errorf("refusing to open %s: it resolves outside the project", rel)
	}
	return real, nil
}

// docContained compares paths rather than strings: filepath.Rel is the check,
// and a leading ".." is the failure.
func docContained(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("outside %s", root)
	}
	return nil
}

// looksBinary decides what a file IS from its bytes rather than its name, so a
// .log full of text reads and a .md that is secretly a PNG does not.
func looksBinary(b []byte) bool {
	if len(b) > docsSniffBytes {
		b = b[:docsSniffBytes]
	}
	if strings.IndexByte(string(b), 0) >= 0 {
		return true
	}
	if utf8.Valid(b) {
		return false
	}
	// The sniff window can end mid-rune, which is not corruption. A UTF-8 rune
	// is at most 4 bytes, so retry without the tail before calling it binary.
	for cut := 1; cut <= 3 && cut < len(b); cut++ {
		if utf8.Valid(b[:len(b)-cut]) {
			return false
		}
	}
	return true
}

// readDocSource returns a document's raw text, or refuses.
func readDocSource(root, rel string) (string, error) {
	path, err := docPath(root, rel)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%s could not be read: %w", rel, err)
	}
	defer f.Close()
	// Enforced WHILE reading, not from a stat: a file can grow between the two,
	// and a declared size is not a promise. The +1 is what separates "at the
	// cap" (readable) from "over it" (refused) — without it an oversized
	// document is silently truncated instead of refused.
	b, err := io.ReadAll(io.LimitReader(f, docsMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("%s could not be read: %w", rel, err)
	}
	if len(b) > docsMaxBytes {
		return "", fmt.Errorf("%s is too large to show here (over %d MB) — open it externally instead",
			rel, docsMaxBytes>>20)
	}
	if looksBinary(b) {
		return "", fmt.Errorf("%s is not a text file — open it externally instead", rel)
	}
	return string(b), nil
}
