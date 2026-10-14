// Package attach turns files the user hands Commander into text a Claude Code
// prompt can carry. Claude Code reads a filesystem PATH out of the prompt, so
// every front-end (drop, paste, screenshot, open dialog) reduces to: produce a
// path, then validate and render it here.
package attach

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

// User-visible reasons; the frontend shows err.Error() verbatim.
var (
	ErrControlChar = errors.New("the name contains a line break or an invisible character")
	ErrNotAbsolute = errors.New("the path is not absolute — it may be part of a name that contains a line break")
	ErrMissing     = errors.New("the file no longer exists")
)

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// isControl is every rune a path may not carry: C0/C1 controls, the line and
// paragraph separators, zero-width characters (which hide text), and bidi
// controls (which reorder it, e.g. "gpj.<RLO>exe" displaying as "exe.jpg").
// All of category Cf (format characters) is refused too: it covers the tag
// characters U+E0000-E007F, soft hyphen U+00AD, invisible math U+2061-2064 and
// U+180E, none of which render. Variation selectors are Mn and stay allowed.
func isControl(r rune) bool {
	switch r {
	case '\u2028', '\u2029', '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
		return true
	}
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || unicode.Is(unicode.Cf, r)
}

// Validate refuses rather than sanitizes. Inside bracketed paste a newline is
// inert, but we cannot observe whether bracketed paste is on, and outside it a
// newline is Enter. Mangling a hostile name would attach a different file than
// the user chose.
//
// The absolute check comes BEFORE anything touches the filesystem and nothing
// is ever resolved against the process cwd: Wails splits a dropped name that
// contains a newline into fragments, and resolving a fragment like "name.pdf"
// would attach whatever file of that name sits in Commander's cwd.
func Validate(p string) (string, error) {
	if !utf8.ValidString(p) {
		return "", ErrControlChar
	}
	for _, r := range p {
		if isControl(r) {
			return "", ErrControlChar
		}
	}
	if p == "" || !filepath.IsAbs(p) {
		return "", ErrNotAbsolute
	}
	clean := filepath.Clean(p)
	if _, err := os.Stat(clean); err != nil {
		return "", ErrMissing
	}
	return clean, nil
}

// IsImage reports whether the extension is one the thumbnail route serves.
func IsImage(path string) bool { return imageExt[strings.ToLower(filepath.Ext(path))] }

// KindOf picks how the confirmation shows an entry.
func KindOf(path string, isDir bool) string {
	switch {
	case isDir:
		return "folder"
	case IsImage(path):
		return "image"
	default:
		return "file"
	}
}

// DisplayName is the base name for messages and the confirmation, with every
// refused character shown as "?" so a hostile name cannot break, reorder or
// hide inside the UI that reports it.
func DisplayName(p string) string {
	if p == "" {
		return "(no name)"
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return '?'
		}
		return r
	}, filepath.Base(p))
}

// PromptText renders an absolute path the way it should appear in the prompt:
// relative ("./x") when inside cwd, escaped the way a terminal escapes a
// dragged-in file, which is the form Claude Code already understands.
func PromptText(abs, cwd string) string {
	out := abs
	if cwd != "" {
		if rel, err := filepath.Rel(cwd, abs); err == nil && rel != "." && !filepath.IsAbs(rel) &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			out = "." + string(filepath.Separator) + rel
		}
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsRune(out, ' ') {
			return `"` + out + `"`
		}
		return out
	}
	var b strings.Builder
	for _, r := range out {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._/@+=:,%-", r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
