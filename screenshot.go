package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
)

// ScreenshotSupport reports whether this platform has a grabber; the UI hides
// the screenshot actions rather than showing an error where it does not.
func (a *App) ScreenshotSupport() bool { return captureRegion != nil }

// CaptureScreenshot runs the interactive region grabber into the attachments
// folder and returns the file's path. "No file written" is a cancel ("", nil)
// whatever the exit status: Esc and a denied permission both end that way.
// NewCapturePath reserves dest with an EMPTY placeholder, so "no file written"
// means missing or zero size, and the placeholder is removed.
func (a *App) CaptureScreenshot() (string, error) {
	if captureRegion == nil {
		return "", errors.New("screen capture is not available on this platform")
	}
	dest, err := a.attachments().NewCapturePath()
	if err != nil {
		return "", err
	}
	_ = captureRegion(dest)
	if fi, err := os.Stat(dest); err != nil || fi.Size() == 0 {
		_ = os.Remove(dest)
		return "", nil
	}
	return dest, nil
}

// LastScreenshot copies the newest system screenshot into the attachments
// folder and returns the copy's path — zero new muscle memory for anyone who
// already hits ⌘⇧4, and an ASCII path that survives Desktop tidying.
func (a *App) LastScreenshot() (string, error) {
	if screenshotDir == nil {
		return "", errors.New("screenshots are not available on this platform")
	}
	src, err := newestScreenshot(screenshotDir(), isScreenCapture)
	if err != nil {
		return "", err
	}
	return a.attachments().CopyIn(src)
}

// newestScreenshot picks the newest image the system marked as a screen
// capture (isCapture: the kMDItemIsScreenCapture xattr on macOS, which works
// for every language and any rename), falling back to the newest
// "Screenshot*"/"Screen Shot*" image when none is marked.
func newestScreenshot(dir string, isCapture func(string) bool) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read the screenshot folder %s", dir)
	}
	var tagged, named string
	var taggedMod, namedMod int64
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !attach.IsImage(name) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		p, m := filepath.Join(dir, name), fi.ModTime().UnixNano()
		if isCapture(p) && (tagged == "" || m > taggedMod) {
			tagged, taggedMod = p, m
		}
		if (strings.HasPrefix(name, "Screenshot") || strings.HasPrefix(name, "Screen Shot")) && (named == "" || m > namedMod) {
			named, namedMod = p, m
		}
	}
	switch {
	case tagged != "":
		return tagged, nil
	case named != "":
		return named, nil
	}
	return "", fmt.Errorf("no screenshots found in %s", dir)
}

// expandTilde expands the "~" and "~/..." forms `defaults` stores for the
// screenshot location.
func expandTilde(d, home string) string {
	switch {
	case d == "~":
		return home
	case strings.HasPrefix(d, "~/"):
		return filepath.Join(home, d[2:])
	}
	return d
}

// bplistTrue reports whether b is a binary plist whose top object is boolean
// true (0x09) or the integer 1 (0x10 0x01): the encodings Spotlight metadata
// uses for a flag such as kMDItemIsScreenCapture.
func bplistTrue(b []byte) bool {
	if len(b) < 10 || string(b[:8]) != "bplist00" {
		return false
	}
	return b[8] == 0x09 || (b[8] == 0x10 && b[9] == 0x01)
}
