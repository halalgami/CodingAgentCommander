package attach

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"
)

// MaxSide is the longest edge Claude accepts; larger images are downscaled.
const MaxSide = 8000

// MaxConvertInputBytes is a sanity cap on a file we will hand to a converter
// (a camera TIFF is routinely far over 20 MB). The result is still held to
// MaxImageBytes, so the prompt never gets an oversize image.
const MaxConvertInputBytes = 200 << 20

// ConvertFunc converts src into dst (an existing empty file, overwritten) as
// format "jpeg" or "png". It must only ever DOWNscale: when either edge of the
// source exceeds maxSide the result is shrunk to fit, otherwise the size is
// kept.
type ConvertFunc func(src, dst, format string, maxSide int) error

// NoConverterError says the platform has nothing that can read the format.
type NoConverterError struct{ Hint string }

func (e *NoConverterError) Error() string { return "no converter is available — " + e.Hint }

// Image formats by sniffed magic. The first four are what Claude reads.
var nativeFormat = map[string]bool{"png": true, "jpeg": true, "gif": true, "webp": true}

var extFormat = map[string]string{
	".png": "png", ".jpg": "jpeg", ".jpeg": "jpeg", ".gif": "gif", ".webp": "webp",
	".heic": "heic", ".heif": "heif", ".avif": "avif",
	".tif": "tiff", ".tiff": "tiff", ".bmp": "bmp", ".ico": "ico",
}

var formatLabel = map[string]string{
	"heic": "HEIC", "heif": "HEIF", "avif": "AVIF", "tiff": "TIFF", "bmp": "BMP", "ico": "ICO",
	"png": "PNG", "jpeg": "JPEG", "gif": "GIF", "webp": "WebP",
}

// sniff reads the leading bytes and names the image format, or "".
func sniff(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 32)
	n, _ := io.ReadFull(f, b)
	b = b[:n]
	switch {
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpeg"
	case len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return "gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	case len(b) >= 4 && (string(b[:4]) == "II*\x00" || string(b[:4]) == "MM\x00*"):
		return "tiff"
	case isBMP(b):
		return "bmp"
	case isICO(b):
		return "ico"
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		switch string(b[8:12]) {
		case "avif", "avis":
			return "avif"
		case "heic", "heix", "hevc", "hevx", "heim", "heis":
			return "heic"
		case "mif1", "msf1":
			return "heif" // generic HEIF brand: HEIC or AVIF, the converter decides
		}
	}
	return ""
}

// isBMP and isICO need a validated header, not just two or four magic bytes:
// "BM" opens plenty of text files ("BMW is...") and a 00 00 01 00 prefix is
// cheap to hit by accident.
func isBMP(b []byte) bool {
	if len(b) < 18 || b[0] != 'B' || b[1] != 'M' {
		return false
	}
	switch binary.LittleEndian.Uint32(b[14:18]) { // DIB header size
	case 12, 40, 52, 56, 108, 124:
		return true
	}
	return false
}

func isICO(b []byte) bool {
	return len(b) >= 6 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0 && binary.LittleEndian.Uint16(b[4:6]) > 0
}

// imageFormat is the format a file is, by content when it can be told and by
// extension otherwise; "" means "not an image we recognise".
func imageFormat(path string) string {
	if f := sniff(path); f != "" {
		return f
	}
	return extFormat[strings.ToLower(filepath.Ext(path))]
}

// dims probes the pixel size without decoding. ok is false when it cannot be
// told (the converter then gets the final say).
func dims(path, format string) (w, h int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	var cfg image.Config
	switch format {
	case "png":
		cfg, err = png.DecodeConfig(f)
	case "jpeg":
		cfg, err = jpeg.DecodeConfig(f)
	case "gif":
		cfg, err = gif.DecodeConfig(f)
	case "webp":
		cfg, err = webp.DecodeConfig(f)
	default:
		return 0, 0, false
	}
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// Normalize is the one stage every attachment passes through (after Validate,
// before PromptText). Non-images and images Claude already reads pass through
// untouched with no copy. Otherwise:
//   - an image over MaxImageBytes is refused, whatever its source;
//   - a format Claude cannot read (HEIC, AVIF, TIFF, BMP, ICO) is converted
//     into a new convert-* file in Dir: JPEG for HEIC/AVIF (photos), PNG for the
//     rest (they may carry alpha);
//   - a readable image whose longest side exceeds MaxSide is downscaled the same
//     way, keeping JPEG as JPEG and everything else as PNG.
//
// The original is never modified. The returned path is what goes in the prompt.
func (s *Store) Normalize(src string, conv ConvertFunc) (string, error) {
	fi, err := os.Stat(src)
	if err != nil || !fi.Mode().IsRegular() {
		return src, nil // directories and odd files are not ours to judge
	}
	format := imageFormat(src)
	if format == "" {
		return src, nil
	}
	// Formats Claude reads go in as-is, so the file itself is bounded by the
	// image limit; a convertible one is judged by its converted size instead.
	if nativeFormat[format] && fi.Size() > MaxImageBytes {
		return "", tooBig(fi.Size())
	}
	if !nativeFormat[format] && fi.Size() > MaxConvertInputBytes {
		return "", fmt.Errorf("the %s image is %d MB; the most that can be converted is %d MB", formatLabel[format], fi.Size()>>20, MaxConvertInputBytes>>20)
	}
	target := ""
	if !nativeFormat[format] {
		target = "png"
		if format == "heic" || format == "heif" || format == "avif" {
			target = "jpeg"
		}
	} else if w, h, ok := dims(src, format); ok && (w > MaxSide || h > MaxSide) {
		target = "png"
		if format == "jpeg" {
			target = "jpeg"
		}
	}
	if target == "" {
		return src, nil
	}
	label := formatLabel[format]
	if conv == nil {
		return "", fmt.Errorf("%s images need conversion and %w", label, &NoConverterError{Hint: "this platform has no converter"})
	}
	ext := ".png"
	if target == "jpeg" {
		ext = ".jpg"
	}
	dst, err := s.reserve("convert", ext)
	if err != nil {
		return "", err
	}
	if err := conv(src, dst, target, MaxSide); err != nil {
		os.Remove(dst)
		var nc *NoConverterError
		if errors.As(err, &nc) {
			return "", fmt.Errorf("%s images need conversion and %w", label, err)
		}
		return "", fmt.Errorf("%s image could not be converted: %w", label, err)
	}
	out, err := os.Stat(dst)
	if err != nil || out.Size() == 0 {
		os.Remove(dst)
		return "", fmt.Errorf("%s image could not be converted: the converter wrote nothing", label)
	}
	if out.Size() > MaxImageBytes {
		os.Remove(dst)
		return "", tooBig(out.Size())
	}
	// Trust the converter's output no further than we can read it: a converter
	// that ignored maxSide (heif-convert never resizes) must not slip a
	// too-large image through.
	w, h, ok := dims(dst, target)
	if !ok {
		os.Remove(dst)
		return "", fmt.Errorf("%s image could not be converted: the result is not a readable image", label)
	}
	if w > MaxSide || h > MaxSide {
		os.Remove(dst)
		return "", fmt.Errorf("%s image is %d x %d px, over the %d px limit, and the converter could not shrink it", label, w, h, MaxSide)
	}
	return dst, nil
}
