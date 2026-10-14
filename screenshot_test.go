package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func mkAged(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("PNG:"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Add(-age)
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
	return p
}

func never(string) bool { return false }

// macOS 14+ names: "Screenshot 2026-10-03 at 10.00.00<U+202F>AM.png".
const macos14Name = "Screenshot 2026-10-03 at 10.00.00 AM.png"

func TestNewestScreenshotFallsBackToTheNamePrefix(t *testing.T) {
	dir := t.TempDir()
	mkAged(t, dir, "Screenshot 2026-10-01 at 10.00.00.png", 3*time.Hour)
	mkAged(t, dir, "Screen Shot 2026-10-02 at 10.00.00 AM.png", 2*time.Hour)
	mkAged(t, dir, macos14Name, 1*time.Hour)
	mkAged(t, dir, "notes.png", 1*time.Minute)                                 // newer, not a screenshot
	mkAged(t, dir, "Screen Recording 2026-10-04 at 10.00.00.mov", time.Second) // newer, a recording
	got, err := newestScreenshot(dir, never)
	if err != nil || filepath.Base(got) != macos14Name {
		t.Errorf("newestScreenshot = %q, %v", got, err)
	}
}

func TestNewestScreenshotPrefersTheScreenCaptureFlag(t *testing.T) {
	dir := t.TempDir()
	tagged := mkAged(t, dir, "Bildschirmfoto 2026-10-03.png", 2*time.Hour) // a localized name
	mkAged(t, dir, "Screenshot 2026-10-04 at 09.00.00.png", time.Hour)     // newer, but untagged
	got, err := newestScreenshot(dir, func(p string) bool { return p == tagged })
	if err != nil || got != tagged {
		t.Errorf("newestScreenshot = %q, %v; want the tagged file", got, err)
	}
}

func TestNewestScreenshotEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := newestScreenshot(dir, never); err == nil || err.Error() != "no screenshots found in "+dir {
		t.Errorf("err = %v", err)
	}
}

func TestExpandTilde(t *testing.T) {
	cases := map[string]string{"~": "/home/u", "~/Pictures/Shots": filepath.Join("/home/u", "Pictures/Shots"), "/abs/dir": "/abs/dir", "": ""}
	for in, want := range cases {
		if got := expandTilde(in, "/home/u"); got != want {
			t.Errorf("expandTilde(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBplistTrue(t *testing.T) {
	trueBool := append([]byte("bplist00\x09\x08"), make([]byte, 32)...)
	falseBool := append([]byte("bplist00\x08\x08"), make([]byte, 32)...)
	intOne := append([]byte("bplist00\x10\x01\x08"), make([]byte, 32)...)
	if !bplistTrue(trueBool) || !bplistTrue(intOne) || bplistTrue(falseBool) || bplistTrue([]byte("1")) || bplistTrue(nil) {
		t.Error("bplistTrue misread a flag")
	}
}

func withCapture(t *testing.T, f func(dest string) error) {
	orig := captureRegion
	captureRegion = f
	t.Cleanup(func() { captureRegion = orig })
}

func TestCaptureScreenshotReturnsTheWrittenFile(t *testing.T) {
	a, _ := newAttachApp(t)
	withCapture(t, func(dest string) error { return os.WriteFile(dest, []byte("PNG"), 0o600) })
	p, err := a.CaptureScreenshot()
	if err != nil || filepath.Dir(p) != a.attachStore.Dir || !regexp.MustCompile(`^capture-\d{8}-\d{6}\.\d{3}\.png$`).MatchString(filepath.Base(p)) {
		t.Errorf("capture = %q, %v", p, err)
	}
}

// Esc exits 0 on some macOS versions and non-zero on others, and a denied
// Screen Recording permission can exit non-zero too: "no file written" is
// the only reliable cancel signal. NewCapturePath reserves the destination
// with an EMPTY placeholder, so "no file written" means missing OR zero size,
// and the placeholder must not be left behind.
func TestCaptureScreenshotWithNoFileIsACancel(t *testing.T) {
	for name, f := range map[string]func(string) error{
		"exit 0":           func(string) error { return nil },
		"exit 1":           func(string) error { return errors.New("exit status 1") },
		"empty file":       func(dest string) error { return os.WriteFile(dest, nil, 0o600) },
		"placeholder kept": func(string) error { return nil }, // grabber writes nothing at all
		"removed by tool":  func(dest string) error { return os.Remove(dest) },
	} {
		t.Run(name, func(t *testing.T) {
			a, _ := newAttachApp(t)
			withCapture(t, f)
			if p, err := a.CaptureScreenshot(); p != "" || err != nil {
				t.Errorf("cancel = %q, %v", p, err)
			}
			if entries, _ := os.ReadDir(a.attachStore.Dir); len(entries) != 0 {
				t.Errorf("left %d files behind", len(entries))
			}
		})
	}
}

func TestCaptureScreenshotUnsupportedPlatform(t *testing.T) {
	a, _ := newAttachApp(t)
	withCapture(t, nil)
	if p, err := a.CaptureScreenshot(); p != "" || err == nil || err.Error() != "screen capture is not available on this platform" {
		t.Errorf("= %q, %v", p, err)
	}
	if a.ScreenshotSupport() {
		t.Error("ScreenshotSupport true with no grabber")
	}
}

func TestLastScreenshotCopiesIntoAttachments(t *testing.T) {
	a, _ := newAttachApp(t)
	shots := t.TempDir()
	src := mkAged(t, shots, macos14Name, time.Minute)
	origDir, origFlag := screenshotDir, isScreenCapture
	screenshotDir, isScreenCapture = func() string { return shots }, never
	t.Cleanup(func() { screenshotDir, isScreenCapture = origDir, origFlag })

	p, err := a.LastScreenshot()
	if err != nil || filepath.Dir(p) != a.attachStore.Dir || !regexp.MustCompile(`^capture-\d{8}-\d{6}\.\d{3}\.png$`).MatchString(filepath.Base(p)) {
		t.Fatalf("LastScreenshot = %q, %v", p, err)
	}
	if got, _ := os.ReadFile(p); string(got) != "PNG:"+macos14Name {
		t.Errorf("copy holds %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("the original screenshot was moved or deleted")
	}
}
