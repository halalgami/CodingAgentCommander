//go:build darwin

package main

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/attach"
	"golang.org/x/image/tiff"
)

// Runs the real macOS sips on a tiny generated file.
func TestSipsConvertsARealTIFFAndDownscalesARealPNG(t *testing.T) {
	if _, err := exec.LookPath("sips"); err != nil {
		t.Skip("no sips")
	}
	d := t.TempDir()
	s := &attach.Store{Dir: filepath.Join(d, "att")}

	tf := filepath.Join(d, "pic.tiff")
	f, _ := os.Create(tf)
	if err := tiff.Encode(f, image.NewRGBA(image.Rect(0, 0, 3, 2)), nil); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out, err := s.Normalize(tf, convertImage)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); len(b) < 8 || string(b[1:4]) != "PNG" {
		t.Errorf("%s is not a PNG", out)
	}

	wide := filepath.Join(d, "wide.png")
	f, _ = os.Create(wide)
	png.Encode(f, image.NewGray(image.Rect(0, 0, 8100, 2)))
	f.Close()
	out, err = s.Normalize(wide, convertImage)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := os.Open(out)
	defer g.Close()
	cfg, err := png.DecodeConfig(g)
	if err != nil || cfg.Width != 8000 {
		t.Errorf("downscaled to %+v, %v", cfg, err)
	}

	// a small image is never enlarged
	small := writePNG(t, filepath.Join(d, "small.png"), 10, 5)
	_ = small
	dst := filepath.Join(d, "small.jpg")
	if err := convertImage(small, dst, "jpeg", attach.MaxSide); err != nil {
		t.Fatal(err)
	}
	if w, h := sipsSize(dst); w != 10 || h != 5 {
		t.Errorf("small image resized to %dx%d", w, h)
	}
	if b, _ := os.ReadFile(dst); len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Error("not a JPEG")
	}
}
