//go:build darwin

package main

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Writes the same xattr screencapture writes (a binary plist holding true) and
// checks the real reader prefers that file over a newer "Screenshot" name.
func TestIsScreenCaptureReadsTheRealXattr(t *testing.T) {
	dir := t.TempDir()
	tagged := mkAged(t, dir, "clip.png", 2*time.Hour)
	mkAged(t, dir, "Screenshot 2026-10-04 at 09.00.00.png", time.Hour)
	val := append([]byte("bplist00\x09\x08"), 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9)
	if err := unix.Setxattr(tagged, "com.apple.metadata:kMDItemIsScreenCapture", val, 0); err != nil {
		t.Skipf("cannot set xattrs here: %v", err)
	}
	if !isScreenCapture(tagged) {
		t.Fatal("isScreenCapture missed the flag")
	}
	got, err := newestScreenshot(dir, isScreenCapture)
	if err != nil || got != tagged {
		t.Errorf("newestScreenshot = %q, %v; want %q", got, err, tagged)
	}
	if _, err := os.Stat(tagged); err != nil {
		t.Fatal(err)
	}
}
