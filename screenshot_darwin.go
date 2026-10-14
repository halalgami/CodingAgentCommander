//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// -i interactive region/window, -x no shutter sound, -t png regardless of the
// user's configured screenshot format. Its exit status is not trusted; see
// CaptureScreenshot.
var captureRegion = func(dest string) error {
	return exec.Command("screencapture", "-i", "-x", "-t", "png", dest).Run()
}

var screenshotDir = func() string {
	home, _ := os.UserHomeDir()
	if out, err := exec.Command("defaults", "read", "com.apple.screencapture", "location").Output(); err == nil {
		if d := expandTilde(strings.TrimSpace(string(out)), home); d != "" {
			return d
		}
	}
	return filepath.Join(home, "Desktop")
}

// isScreenCapture reads the flag screencapture sets on every file it writes.
// No cgo: x/sys/unix calls getxattr(2) directly.
var isScreenCapture = func(path string) bool {
	buf := make([]byte, 128)
	n, err := unix.Getxattr(path, "com.apple.metadata:kMDItemIsScreenCapture", buf)
	return err == nil && bplistTrue(buf[:n])
}
