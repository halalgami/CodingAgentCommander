package attach

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MaxImageBytes caps a pasted image. The bytes cross the Wails bridge as
// base64, so this also bounds that payload.
const MaxImageBytes = 20 << 20

var mimeExt = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	// Not readable by Claude: saved as-is, then converted by Normalize.
	"image/heic": ".heic", "image/heif": ".heif", "image/avif": ".avif",
	"image/tiff": ".tiff", "image/bmp": ".bmp", "image/x-icon": ".ico", "image/vnd.microsoft.icon": ".ico",
}

// reapable matches ONLY names this package writes: paste-<date>-…,
// capture-<date>-…, convert-<date>-…, and the .paste-* temp files a crash can strand. Anything
// else in the folder — a file the user put there by hand — is never deleted.
var reapable = regexp.MustCompile(`^(?:(?:paste|capture|convert)-\d{8}-|\.paste-)`)

// Store owns the files Commander itself creates (pastes, captures, copied-in
// screenshots). Dropped files are never copied here, so they are never reaped:
// only bytes this app wrote are its to delete.
type Store struct {
	Dir string
	Now func() time.Time
}

// DefaultDir is ~/.config/commander/attachments on every platform. Not /tmp
// (macOS purges it after ~3 days idle, but a path already sent into a
// conversation must stay resolvable) and not os.UserConfigDir() (the app's
// convention is ~/.config; see internal/config/paths.go).
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "commander", "attachments")
	}
	return filepath.Join(home, ".config", "commander", "attachments")
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// writeAtomic streams r into a temp file in Dir and renames it to final, so a
// reader never sees a half-written attachment.
func (s *Store) writeAtomic(final string, r io.Reader) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".paste-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// tooBig reports an oversize image with KB precision, so a file a few bytes
// over the cap never reads "is 20 MB; the limit is 20 MB".
func tooBig(n int64) error {
	return fmt.Errorf("the image is %d KB; the limit is %d MB", (n+1023)>>10, MaxImageBytes>>20)
}

// SaveImage writes clipboard image bytes atomically and returns the path.
func (s *Store) SaveImage(data []byte, mime string) (string, error) {
	ext, ok := mimeExt[strings.ToLower(mime)]
	if !ok {
		label := mime
		if label == "" {
			label = "unknown"
		}
		return "", fmt.Errorf("%s images are not supported — use PNG, JPEG, GIF or WebP", label)
	}
	if len(data) == 0 {
		return "", errors.New("the clipboard image is empty")
	}
	if len(data) > MaxImageBytes {
		return "", tooBig(int64(len(data)))
	}
	sum := sha256.Sum256(data)
	final := filepath.Join(s.Dir, fmt.Sprintf("paste-%s-%s%s", s.now().Format("20060102-150405"), hex.EncodeToString(sum[:4]), ext))
	if err := s.writeAtomic(final, bytes.NewReader(data)); err != nil {
		return "", err
	}
	return final, nil
}

// reserveCapture atomically creates an empty capture-<ts>[-n]<ext> file and
// returns its path, so two calls in the same millisecond (or a leftover file)
// can never be handed the same name. The caller overwrites it.
func (s *Store) reserveCapture(ext string) (string, error) { return s.reserve("capture", ext) }

func (s *Store) reserve(prefix, ext string) (string, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	base := prefix + "-" + s.now().Format("20060102-150405.000")
	for i := 0; i < 1000; i++ {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		p := filepath.Join(s.Dir, name)
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return p, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not find a free %s file name", prefix)
}

// NewPasteBase returns a fresh extension-less destination (paste-<ts>-clip-<rand>)
// for a native clipboard reader that writes <base>.png or <base>.tiff itself.
// The name is a paste-* one, so Reap deletes it after a week.
func (s *Store) NewPasteBase() (string, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	var r [4]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, fmt.Sprintf("paste-%s-clip-%s", s.now().Format("20060102-150405"), hex.EncodeToString(r[:]))), nil
}

// NewCapturePath reserves a destination for a screen capture (the grabber
// overwrites the empty placeholder file; when the capture is cancelled the
// caller, CaptureScreenshot, removes the placeholder — Reap is only the net for
// one left behind by a crash).
func (s *Store) NewCapturePath() (string, error) { return s.reserveCapture(".png") }

// CopyIn copies an image Commander did not create (the newest system
// screenshot) into Dir under an ASCII capture- name. The path that goes into
// the prompt then survives Desktop tidying, and needs no escaping for the
// U+202F that macOS 14 puts before "AM"/"PM" in screenshot names.
func (s *Store) CopyIn(src string) (string, error) {
	if !IsImage(src) {
		return "", fmt.Errorf("%s is not an image", DisplayName(src))
	}
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", DisplayName(src))
	}
	if fi.Size() > MaxImageBytes {
		return "", tooBig(fi.Size())
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	final, err := s.reserveCapture(strings.ToLower(filepath.Ext(src)))
	if err != nil {
		return "", err
	}
	if err := s.writeAtomic(final, in); err != nil {
		os.Remove(final)
		return "", err
	}
	return final, nil
}

// Reap deletes this package's own files older than maxAge. Age-based, not
// session-scoped: a path sent into a conversation must survive as long as that
// conversation might be revisited. It refuses to work through a symlinked Dir
// and skips symlinked entries (DirEntry.Type is the Lstat type), so it can
// never delete outside the folder.
func (s *Store) Reap(maxAge time.Duration) (int, error) {
	fi, err := os.Lstat(s.Dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to reap %s: it is a symlink", s.Dir)
	}
	if !fi.IsDir() {
		return 0, fmt.Errorf("refusing to reap %s: not a directory", s.Dir)
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0, err
	}
	cutoff := s.now().Add(-maxAge)
	n := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !reapable.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(s.Dir, e.Name())) == nil {
			n++
		}
	}
	return n, nil
}
