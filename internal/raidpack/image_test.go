package raidpack

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	im.Set(0, 0, color.NRGBA{255, 0, 0, 0})       // transparent pixel
	im.Set(w-1, h-1, color.NRGBA{0, 255, 0, 255}) // far corner: scaling must not bleed it into (0,0)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStoreScalesKeepsAlphaAndReplacesOtherExt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "opus-stance.jpg", "old")
	if err := Store(dir, "opus-stance", bytes.NewReader(pngBytes(t, 1000, 2000))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opus-stance.jpg")); !os.IsNotExist(err) {
		t.Fatal("old .jpg of the same stem must be removed")
	}
	f, _ := os.Open(filepath.Join(dir, "opus-stance.png"))
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := im.Bounds(); b.Dy() != MaxH || b.Dx() != 420 {
		t.Fatalf("size %v, want 420x840", b)
	}
	if _, _, _, a := im.At(0, 0).RGBA(); a != 0 {
		t.Fatal("alpha lost")
	}
}

func TestStoreNeverUpscales(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "kimi-hurt", bytes.NewReader(pngBytes(t, 100, 200))); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(filepath.Join(dir, "kimi-hurt.png"))
	defer f.Close()
	cfg, _ := png.DecodeConfig(f)
	if cfg.Height != 200 {
		t.Fatalf("upscaled to %d", cfg.Height)
	}
}

func TestNormalizeRefusesHuge(t *testing.T) {
	dir := t.TempDir()
	err := Store(dir, "opus-stance", bytes.NewReader(pngBytes(t, 10, 9000)))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want too large, got %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatal("nothing may be written for a refused image")
	}
}

func TestStoreRefusesBadStemAndNonImage(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "../x", bytes.NewReader(pngBytes(t, 4, 4))); err == nil {
		t.Fatal("bad stem accepted")
	}
	if err := Store(dir, "opus-stance", strings.NewReader("not an image")); err == nil {
		t.Fatal("non-image accepted")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "glm-await.png", "a")
	write(t, dir, "glm-await.webp", "b")
	if err := Remove(dir, "glm-await"); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("left %v", ents)
	}
	if err := Remove(dir, "nope"); err == nil {
		t.Fatal("bad stem accepted")
	}
}

// pngHeader is a PNG with only its IHDR: enough for DecodeConfig, no pixels.
func pngHeader(w, h int, depth, colorType byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(typ string, data []byte) {
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		c := crc32.NewIEEE()
		c.Write([]byte(typ))
		c.Write(data)
		b.WriteString(typ)
		b.Write(data)
		binary.Write(&b, binary.BigEndian, c.Sum32())
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8], ihdr[9] = depth, colorType
	chunk("IHDR", ihdr)
	chunk("IEND", nil)
	return b.Bytes()
}

// Minors sweep: 8000 × 8000 at 16 bits passed the side check but decodes to
// ~512 MB. The cap is on decoded memory, so an ordinary 8-bit phone photo
// (48 MP) still passes and only deep-colour giants are refused.
func TestNormalizeCapsDecodedMemoryNotPhotos(t *testing.T) {
	err := Store(t.TempDir(), "opus-stance", bytes.NewReader(pngHeader(6000, 6000, 16, 6)))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("16-bit 36 MP: want too large, got %v", err)
	}
	err = Store(t.TempDir(), "opus-stance", bytes.NewReader(pngHeader(8000, 6000, 8, 6)))
	if err != nil && strings.Contains(err.Error(), "too large") {
		t.Fatalf("8-bit 48 MP photo refused as too large: %v", err)
	}
}
