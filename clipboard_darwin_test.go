//go:build darwin

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
	"golang.org/x/image/tiff"
)

// A private NAMED pasteboard: these tests never touch the user's general one.
const seedScript = `function run(argv) {
ObjC.import('AppKit');
var pb=$.NSPasteboard.pasteboardWithName($(argv[0]));
pb.clearContents;
var urls=[];
for (var i=1;i<argv.length;i++) {
  var kv=argv[i].split('|'); var t=kv[0]; var p=kv.slice(1).join('|');
  if (t==='url') urls.push($.NSURL.fileURLWithPath($(p)));
  else pb.setDataForType($.NSData.dataWithContentsOfFile($(p)), $(t));
}
if (urls.length) pb.writeObjects($(urls));
return 'ok';
}`

const releaseScript = `function run(argv) { ObjC.import('AppKit'); $.NSPasteboard.pasteboardWithName($(argv[0])).releaseGlobally; return 'ok'; }`

func seededPasteboard(t *testing.T, entries ...string) string {
	t.Helper()
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skip("no osascript")
	}
	var r [6]byte
	rand.Read(r[:])
	name := "cc-test-" + hex.EncodeToString(r[:])
	t.Cleanup(func() { exec.Command("osascript", "-l", "JavaScript", "-e", releaseScript, name).Run() })
	args := append([]string{"-l", "JavaScript", "-e", seedScript, name}, entries...)
	if out, err := exec.Command("osascript", args...).CombinedOutput(); err != nil {
		t.Fatalf("seeding %s: %v: %s", name, err, out)
	}
	return name
}

func pngFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, _ := os.Create(p)
	png.Encode(f, image.NewGray(image.Rect(0, 0, 4, 4)))
	f.Close()
	if size > 0 {
		g, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
		g.Write(make([]byte, size))
		g.Close()
	}
	return p
}

func tiffFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, _ := os.Create(p)
	tiff.Encode(f, image.NewGray(image.Rect(0, 0, 2, 2)), nil)
	f.Close()
	return p
}

func TestPasteboardFileURLsWinOverImageData(t *testing.T) {
	d := t.TempDir()
	file := touch(t, filepath.Join(d, "my doc.pdf"))
	name := seededPasteboard(t, "url|"+file, "public.png|"+pngFile(t, d, "p.png", 0))
	c, err := readPasteboard(filepath.Join(d, "base"), name)
	if err != nil || len(c.Files) != 1 || c.Files[0] != file || c.Image != "" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := os.Stat(filepath.Join(d, "base.png")); err == nil {
		t.Error("an image was written although files were present")
	}
}

func TestPasteboardPrefersPNGOverTIFF(t *testing.T) {
	d := t.TempDir()
	name := seededPasteboard(t, "public.tiff|"+tiffFile(t, d, "t.tiff"), "public.png|"+pngFile(t, d, "p.png", 0))
	c, err := readPasteboard(filepath.Join(d, "base"), name)
	if err != nil || len(c.Files) != 0 || c.Image != filepath.Join(d, "base.png") {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := png.DecodeConfig(mustOpen(t, c.Image)); err != nil {
		t.Errorf("the written image is not a PNG: %v", err)
	}
}

func TestPasteboardFallsBackToTIFF(t *testing.T) {
	d := t.TempDir()
	name := seededPasteboard(t, "public.tiff|"+tiffFile(t, d, "t.tiff"))
	c, err := readPasteboard(filepath.Join(d, "base"), name)
	if err != nil || c.Image != filepath.Join(d, "base.tiff") {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestPasteboardNewlinePathRoundTripsAndValidateRefusesIt(t *testing.T) {
	d := t.TempDir()
	odd := touch(t, filepath.Join(d, "a\nb.txt"))
	name := seededPasteboard(t, "url|"+odd)
	c, err := readPasteboard(filepath.Join(d, "base"), name)
	if err != nil || len(c.Files) != 1 || c.Files[0] != odd {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := attach.Validate(c.Files[0]); err == nil {
		t.Error("Validate must refuse a name with a line break")
	}
}

// The pasteboard synthesizes a TIFF for any PNG it holds, so an oversize PNG
// is never written (20 MB cap) and the read falls back to the TIFF. The
// tooBig outcome itself needs both representations over their caps and is
// covered through the Go seam (TestPasteClipboardTooBigImageIsRefusedWithItsSize).
func TestPasteboardOversizePNGIsNotWrittenAndFallsBackToTIFF(t *testing.T) {
	d := t.TempDir()
	name := seededPasteboard(t, "public.png|"+pngFile(t, d, "big.png", 21<<20), "public.tiff|"+tiffFile(t, d, "t.tiff"))
	c, err := readPasteboard(filepath.Join(d, "base"), name)
	if err != nil || c.Image != filepath.Join(d, "base.tiff") || c.TooBig != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := os.Stat(filepath.Join(d, "base.png")); err == nil {
		t.Error("the oversize PNG was written")
	}
}

func mustOpen(t *testing.T, p string) *os.File {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(fmt.Sprint(err))
	}
	t.Cleanup(func() { f.Close() })
	return f
}
