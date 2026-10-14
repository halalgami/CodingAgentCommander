package raidpack

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	MaxH    = 840
	MaxSide = 8000
	// MaxDecode caps decoded memory: 8000 × 8000 passes MaxSide but is ~512 MB
	// as 16-bit RGBA. 256 MB keeps every 8-bit photo (≤ 64 MP) and refuses
	// deep-colour giants.
	MaxDecode = 256 << 20
	MaxFile   = 32 << 20
	maxFile   = MaxFile
)

// bytesPerPixel is what decoding costs per pixel for a colour model.
func bytesPerPixel(m color.Model) int64 {
	switch m {
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	case color.Gray16Model:
		return 2
	case color.GrayModel, color.AlphaModel:
		return 1
	}
	return 4
}

// normalize decodes any supported image, refuses absurd sizes BEFORE
// allocating pixels, and scales it to at most MaxH tall (never up).
func normalize(b []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("that file is not a PNG, JPEG or WebP image")
	}
	if cfg.Width > MaxSide || cfg.Height > MaxSide {
		return nil, fmt.Errorf("image is %dx%d, too large (max %d px a side)", cfg.Width, cfg.Height, MaxSide)
	}
	if int64(cfg.Width)*int64(cfg.Height)*bytesPerPixel(cfg.ColorModel) > MaxDecode {
		return nil, fmt.Errorf("image is %dx%d at 16 bits a channel, too large to load (save it as 8-bit)", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("cannot decode image: %w", err)
	}
	h := src.Bounds().Dy()
	if h <= MaxH {
		return src, nil
	}
	w := src.Bounds().Dx() * MaxH / h
	if w < 1 {
		w = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, MaxH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst, nil
}

// Store normalises r and writes <dir>/<stem>.png atomically, then removes any
// other extension of the same stem so exactly one file provides it.
func Store(dir, stem string, r io.Reader) error {
	if !ValidStem(stem) {
		return fmt.Errorf("%q is not a heroine image name", stem)
	}
	b, err := io.ReadAll(io.LimitReader(r, maxFile+1))
	if err != nil {
		return err
	}
	if len(b) > maxFile {
		return fmt.Errorf("image is over %d MiB", maxFile>>20)
	}
	im, err := normalize(b)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, stem+".png"), out.Bytes()); err != nil {
		return err
	}
	for ext := range ImageExt {
		if ext != ".png" {
			os.Remove(filepath.Join(dir, stem+ext))
		}
	}
	return nil
}

// Remove deletes every file providing stem.
func Remove(dir, stem string) error {
	if !ValidStem(stem) {
		return fmt.Errorf("%q is not a heroine image name", stem)
	}
	for ext := range ImageExt {
		if err := os.Remove(filepath.Join(dir, stem+ext)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
