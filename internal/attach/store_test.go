package attach

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }

func TestSaveImageLeavesNoTempFile(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "attachments"), Now: fixedNow}
	data := []byte("\x89PNG fake")
	p, err := s.SaveImage(data, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^paste-20261004-120000-[0-9a-f]{8}\.png$`).MatchString(filepath.Base(p)) || filepath.Dir(p) != s.Dir {
		t.Errorf("unexpected path %q", p)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, data) {
		t.Error("bytes differ")
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %d entries", len(entries))
	}
}

func TestSaveImageRefusals(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Now: fixedNow}
	if _, err := s.SaveImage([]byte("x"), "image/x-foo"); err == nil || err.Error() != "image/x-foo images are not supported — use PNG, JPEG, GIF or WebP" {
		t.Errorf("unknown formats must be refused naming the format, got %v", err)
	}
	if _, err := s.SaveImage(nil, "image/png"); err == nil || err.Error() != "the clipboard image is empty" {
		t.Errorf("empty image: %v", err)
	}
	if _, err := s.SaveImage(make([]byte, MaxImageBytes+1), "image/png"); err == nil || err.Error() != "the image is 20481 KB; the limit is 20 MB" {
		t.Errorf("oversize image: %v", err)
	}
	if _, err := s.SaveImage(make([]byte, MaxImageBytes), "image/png"); err != nil {
		t.Errorf("exactly MaxImageBytes must be accepted: %v", err)
	}
	for _, m := range []string{"image/jpeg", "image/gif", "image/webp", "IMAGE/PNG"} {
		if _, err := s.SaveImage([]byte("x"), m); err != nil {
			t.Errorf("%s refused: %v", m, err)
		}
	}
}

func TestNewCapturePathIsUnderDirAndPNG(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "a"), Now: fixedNow}
	p, err := s.NewCapturePath()
	if err != nil || filepath.Dir(p) != s.Dir || filepath.Base(p) != "capture-20261004-120000.000.png" {
		t.Errorf("NewCapturePath = %q, %v", p, err)
	}
	if fi, err := os.Stat(s.Dir); err != nil || !fi.IsDir() {
		t.Error("dir not created")
	}
}

func TestCopyInGivesAnASCIICaptureName(t *testing.T) {
	src := filepath.Join(t.TempDir(), "Screenshot 2026-10-03 at 10.00.00\u202fAM.png")
	if err := os.WriteFile(src, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: filepath.Join(t.TempDir(), "a"), Now: fixedNow}
	p, err := s.CopyIn(src)
	if err != nil || filepath.Dir(p) != s.Dir || filepath.Base(p) != "capture-20261004-120000.000.png" {
		t.Fatalf("CopyIn = %q, %v", p, err)
	}
	if got, _ := os.ReadFile(p); string(got) != "PNGDATA" {
		t.Errorf("copy = %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("CopyIn must never move or delete the original")
	}
	if _, err := s.CopyIn(filepath.Join(t.TempDir(), "notes.txt")); err == nil || err.Error() != "notes.txt is not an image" {
		t.Errorf("non-image: %v", err)
	}
}

func age(t *testing.T, p string, d time.Duration) {
	t.Helper()
	ts := fixedNow().Add(-d)
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func TestReapDeletesOnlyOldFilesItCreated(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Now: fixedNow}
	mk := func(name string, d time.Duration) string {
		p := filepath.Join(s.Dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		age(t, p, d)
		return p
	}
	week := 7 * 24 * time.Hour
	gone := []string{
		mk("paste-20260920-101010-0a1b2c3d.png", week+time.Hour),
		mk("capture-20260920-101010.000.png", week+time.Hour),
		mk("convert-20260920-101010.000.jpg", week+time.Hour),
		mk(".paste-123456", week+time.Hour),
	}
	kept := []string{
		mk("paste-20261003-101010-0a1b2c3d.png", week-time.Hour), // fresh
		mk("notes.png", 30*24*time.Hour),                         // foreign name, old
		mk("paste-notes.png", 30*24*time.Hour),                   // prefix but no date
	}
	sub := filepath.Join(s.Dir, "capture-20260901-sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, sub, 30*24*time.Hour)
	kept = append(kept, sub)
	if runtime.GOOS != "windows" {
		target := filepath.Join(t.TempDir(), "victim.png")
		if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(s.Dir, "paste-20260901-101010-deadbeef.png")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, link, target)
	}

	n, err := s.Reap(week)
	if err != nil || n != len(gone) {
		t.Fatalf("Reap = %d, %v; want %d, nil", n, err, len(gone))
	}
	for _, p := range gone {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived", filepath.Base(p))
		}
	}
	for _, p := range kept {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s was deleted", filepath.Base(p))
		}
	}
}

func TestReapRefusesASymlinkedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privilege on Windows")
	}
	real := t.TempDir()
	victim := filepath.Join(real, "paste-20260901-101010-deadbeef.png")
	if err := os.WriteFile(victim, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	age(t, victim, 30*24*time.Hour)
	link := filepath.Join(t.TempDir(), "attachments")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: link, Now: fixedNow}
	n, err := s.Reap(time.Hour)
	if n != 0 || err == nil || err.Error() != "refusing to reap "+link+": it is a symlink" {
		t.Errorf("Reap = %d, %v", n, err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("a file behind the symlinked dir was deleted")
	}
}

func TestReapMissingDirIsNotAnError(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "never-created")}
	if n, err := s.Reap(time.Hour); n != 0 || err != nil {
		t.Errorf("Reap on missing dir = %d, %v", n, err)
	}
}

func TestDefaultDirIsUnderDotConfig(t *testing.T) {
	if !strings.HasSuffix(DefaultDir(), filepath.Join(".config", "commander", "attachments")) {
		t.Errorf("DefaultDir = %q", DefaultDir())
	}
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("boom")
	}
	f.n--
	return copy(p, "partial"), nil
}

func TestWriteAtomicFailureLeavesNothing(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Now: fixedNow}
	final := filepath.Join(s.Dir, "paste-20261004-120000-deadbeef.png")
	if err := s.writeAtomic(final, io.MultiReader(&failingReader{n: 1})); err == nil {
		t.Fatal("expected the read error")
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 0 {
		t.Errorf("failed write left %d entries (final or .paste-* temp)", len(entries))
	}
}

func TestReapSkipsSymlinkEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privilege on Windows")
	}
	s := &Store{Dir: t.TempDir()}
	target := filepath.Join(t.TempDir(), "victim.png")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.Dir, "paste-20260901-101010-deadbeef.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	// Chtimes would age the TARGET, not the link. Instead move "now" far into
	// the future so the link's own (real) mtime is old by any measure.
	s.Now = func() time.Time { return time.Now().Add(60 * 24 * time.Hour) }
	n, err := s.Reap(7 * 24 * time.Hour)
	if n != 0 || err != nil {
		t.Errorf("Reap = %d, %v", n, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Error("an old symlink entry was reaped")
	}
}

func TestNewCapturePathIsUniqueWithinOneInstant(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Now: fixedNow}
	a, err1 := s.NewCapturePath()
	b, err2 := s.NewCapturePath()
	if err1 != nil || err2 != nil || a == b {
		t.Errorf("NewCapturePath twice = %q, %q (%v, %v); names must differ", a, b, err1, err2)
	}
	src := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(src, []byte("x"), 0o600)
	c, err := s.CopyIn(src)
	if err != nil || c == a || c == b {
		t.Errorf("CopyIn collided: %q, %v", c, err)
	}
}

func TestCopyInRefusals(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "a"), Now: fixedNow}
	dir := filepath.Join(t.TempDir(), "d.png")
	os.Mkdir(dir, 0o700)
	if _, err := s.CopyIn(dir); err == nil || err.Error() != "d.png is not a regular file" {
		t.Errorf("directory: %v", err)
	}
	big := filepath.Join(t.TempDir(), "big.png")
	f, _ := os.Create(big)
	f.Truncate(MaxImageBytes + 1)
	f.Close()
	if _, err := s.CopyIn(big); err == nil || err.Error() != "the image is 20481 KB; the limit is 20 MB" {
		t.Errorf("oversize: %v", err)
	}
}
