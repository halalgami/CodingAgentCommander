package attach

import (
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/tiff"
)

type call struct {
	src, dst, format string
	max              int
}

// stubConv records calls and writes a stand-in output so Normalize sees a file.
func stubConv(calls *[]call) ConvertFunc {
	return func(src, dst, format string, max int) error {
		*calls = append(*calls, call{src, dst, format, max})
		return writeImage(dst, format, 1, 1)
	}
}

// writeImage writes a real tiny image: Normalize reads the output's size back.
func writeImage(dst, format string, w, h int) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if format == "jpeg" {
		return jpeg.Encode(f, image.NewGray(image.Rect(0, 0, w, h)), nil)
	}
	return png.Encode(f, image.NewGray(image.Rect(0, 0, w, h)))
}

func put(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	f := filepath.Join(t.TempDir(), "x.png")
	o, _ := os.Create(f)
	if err := png.Encode(o, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	o.Close()
	b, _ := os.ReadFile(f)
	return b
}

func TestNormalizeConvertsEachUnsupportedFormat(t *testing.T) {
	heic := append([]byte{0, 0, 0, 24}, []byte("ftypheic")...)
	avif := append([]byte{0, 0, 0, 24}, []byte("ftypavif")...)
	tif := []byte("II*\x00\x08\x00\x00\x00")
	bmp := []byte("BM\x00\x00\x00\x00\x00\x00")
	ico := []byte("\x00\x00\x01\x00\x01\x00")
	cases := []struct {
		name   string
		b      []byte
		format string
		ext    string
	}{
		{"a.heic", heic, "jpeg", ".jpg"}, {"a.heif", heic, "jpeg", ".jpg"}, {"a.avif", avif, "jpeg", ".jpg"}, {"a.dat", append([]byte{0, 0, 0, 24}, []byte("ftypmif1")...), "jpeg", ".jpg"},
		{"a.tiff", tif, "png", ".png"}, {"a.tif", tif, "png", ".png"},
		{"a.bmp", bmp, "png", ".png"}, {"a.ico", ico, "png", ".png"},
		// content wins over a lying extension
		{"photo.png", heic, "jpeg", ".jpg"},
	}
	for _, c := range cases {
		s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
		src := put(t, t.TempDir(), c.name, c.b)
		var calls []call
		out, err := s.Normalize(src, stubConv(&calls))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(calls) != 1 || calls[0].src != src || calls[0].format != c.format || calls[0].max != MaxSide {
			t.Errorf("%s: calls %+v", c.name, calls)
		}
		if filepath.Dir(out) != s.Dir || !strings.HasPrefix(filepath.Base(out), "convert-") || filepath.Ext(out) != c.ext {
			t.Errorf("%s: out = %s", c.name, out)
		}
		if b, _ := os.ReadFile(src); string(b) != string(c.b) {
			t.Errorf("%s: original modified", c.name)
		}
	}
}

func TestNormalizePassesSupportedAndNonImagesThroughWithoutConverting(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	d := t.TempDir()
	var calls []call
	for _, p := range []string{
		put(t, d, "a.png", pngBytes(t, 4, 4)), put(t, d, "b.pdf", []byte("%PDF")), put(t, d, "c.txt", []byte("hi")), d,
	} {
		out, err := s.Normalize(p, stubConv(&calls))
		if err != nil || out != p {
			t.Errorf("%s: %q, %v", p, out, err)
		}
	}
	if len(calls) != 0 {
		t.Errorf("converter called: %+v", calls)
	}
	if _, err := os.Stat(s.Dir); err == nil {
		t.Error("a pass-through must not even create the store")
	}
}

func TestNormalizeRefusesAnOversizeImageFromAnySource(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	big := put(t, t.TempDir(), "big.png", append(pngBytes(t, 4, 4), make([]byte, MaxImageBytes)...))
	var calls []call
	_, err := s.Normalize(big, stubConv(&calls))
	if err == nil || !strings.Contains(err.Error(), "the limit is 20 MB") || len(calls) != 0 {
		t.Errorf("err = %v calls = %v", err, calls)
	}
}

func TestNormalizeDownscalesAnOverlongImage(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	d := t.TempDir()
	var calls []call
	if out, err := s.Normalize(put(t, d, "wide.png", pngBytes(t, 8001, 1)), stubConv(&calls)); err != nil || filepath.Ext(out) != ".png" {
		t.Fatalf("%q %v", out, err)
	}
	if len(calls) != 1 || calls[0].format != "png" || calls[0].max != 8000 {
		t.Errorf("calls %+v", calls)
	}
	// exactly at the limit is fine
	calls = nil
	p := put(t, d, "edge.png", pngBytes(t, 8000, 1))
	if out, err := s.Normalize(p, stubConv(&calls)); err != nil || out != p || len(calls) != 0 {
		t.Errorf("8000 px must pass: %q %v %v", out, err, calls)
	}
	// a tall JPEG stays JPEG
	calls = nil
	jp := filepath.Join(d, "tall.jpg")
	o, _ := os.Create(jp)
	jpeg.Encode(o, image.NewGray(image.Rect(0, 0, 1, 8001)), nil)
	o.Close()
	if out, err := s.Normalize(jp, stubConv(&calls)); err != nil || filepath.Ext(out) != ".jpg" || calls[0].format != "jpeg" {
		t.Errorf("%q %v %+v", out, err, calls)
	}
}

func TestNormalizeConverterFailureAndMissingConverter(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	src := put(t, t.TempDir(), "a.heic", append([]byte{0, 0, 0, 24}, []byte("ftypheic")...))
	_, err := s.Normalize(src, func(_, _, _ string, _ int) error { return &NoConverterError{Hint: "install libheif"} })
	if err == nil || err.Error() != "HEIC images need conversion and no converter is available — install libheif" {
		t.Errorf("err = %v", err)
	}
	_, err = s.Normalize(src, func(_, _, _ string, _ int) error { return errors.New("boom") })
	if err == nil || err.Error() != "HEIC image could not be converted: boom" {
		t.Errorf("err = %v", err)
	}
	if _, err := s.Normalize(src, nil); err == nil || !strings.HasPrefix(err.Error(), "HEIC images need conversion and no converter is available") {
		t.Errorf("nil converter: %v", err)
	}
	// nothing is left behind by the failures
	if es, _ := os.ReadDir(s.Dir); len(es) != 0 {
		t.Errorf("leftovers: %v", es)
	}
}

func TestNormalizeRefusesAConverterThatWritesNothingOrTooMuch(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	src := put(t, t.TempDir(), "a.tiff", []byte("II*\x00"))
	if _, err := s.Normalize(src, func(_, _, _ string, _ int) error { return nil }); err == nil {
		t.Error("an empty output must be an error")
	}
	_, err := s.Normalize(src, func(_, dst, _ string, _ int) error { return os.WriteFile(dst, make([]byte, MaxImageBytes+1), 0o600) })
	if err == nil || !strings.Contains(err.Error(), "the limit is 20 MB") {
		t.Errorf("err = %v", err)
	}
	if es, _ := os.ReadDir(s.Dir); len(es) != 0 {
		t.Errorf("leftovers: %v", es)
	}
}

func TestSniffRealTIFF(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.dat")
	o, _ := os.Create(p)
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	if err := tiff.Encode(o, img, nil); err != nil {
		t.Fatal(err)
	}
	o.Close()
	if sniff(p) != "tiff" {
		t.Errorf("sniff = %q", sniff(p))
	}
}

func TestNormalizeRefusesAConverterThatIgnoredMaxSide(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	src := put(t, t.TempDir(), "a.heic", append([]byte{0, 0, 0, 24}, []byte("ftypheic")...))
	_, err := s.Normalize(src, func(_, dst, format string, _ int) error { return writeImage(dst, format, 8001, 1) })
	if err == nil || !strings.Contains(err.Error(), "over the 8000 px limit") {
		t.Errorf("err = %v", err)
	}
	if es, _ := os.ReadDir(s.Dir); len(es) != 0 {
		t.Errorf("leftovers: %v", es)
	}
	// an output that is not an image at all is refused too
	_, err = s.Normalize(src, func(_, dst, _ string, _ int) error { return os.WriteFile(dst, []byte("junk"), 0o600) })
	if err == nil || !strings.Contains(err.Error(), "not a readable image") {
		t.Errorf("err = %v", err)
	}
}

func TestNormalizeDoesNotMistakeOrdinaryFilesForBMPOrICO(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	d := t.TempDir()
	var calls []call
	for name, body := range map[string]string{
		"notes.txt":  "BMW is a car maker, not a bitmap header at all.",
		"data.csv":   "BM,1,2\n3,4,5\n",
		"blank.dat":  "\x00\x00\x01\x00\x00\x00 and then text",
		"photo.heic": "this is text pretending to be a HEIC but it is only text",
	} {
		p := put(t, d, name, []byte(body))
		out, err := s.Normalize(p, stubConv(&calls))
		if name == "photo.heic" {
			continue // by extension it is still offered to the converter
		}
		if err != nil || out != p {
			t.Errorf("%s: %q %v", name, out, err)
		}
	}
	if len(calls) != 1 || calls[0].src != filepath.Join(d, "photo.heic") {
		t.Errorf("only the .heic-named file may reach the converter: %+v", calls)
	}
	// the reverse lie: a .heic that is really a PNG is a PNG
	calls = nil
	real := put(t, d, "really.heic", pngBytes(t, 3, 3))
	if out, err := s.Normalize(real, stubConv(&calls)); err != nil || out != real || len(calls) != 0 {
		t.Errorf("a PNG named .heic: %q %v %v", out, err, calls)
	}
	// real headers are still recognised
	bmp := make([]byte, 32)
	copy(bmp, "BM")
	bmp[14] = 40
	ico := []byte{0, 0, 1, 0, 1, 0, 0, 0}
	for name, b := range map[string][]byte{"x.dat": bmp, "y.dat": ico} {
		calls = nil
		if _, err := s.Normalize(put(t, d, name, b), stubConv(&calls)); err != nil || len(calls) != 1 {
			t.Errorf("%s: %v %v", name, err, calls)
		}
	}
}

func TestNormalizeSizeCapAppliesToInputOnlyForNativeFormats(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	d := t.TempDir()
	var calls []call
	// a 25 MB TIFF is converted; the 20 MB rule is applied to the RESULT
	tif := put(t, d, "big.tiff", append([]byte("II*\x00"), make([]byte, 25<<20)...))
	if out, err := s.Normalize(tif, stubConv(&calls)); err != nil || filepath.Ext(out) != ".png" || len(calls) != 1 {
		t.Errorf("25 MB tiff: %q %v %v", out, err, calls)
	}
	// ...and a convertible input over the sanity cap is refused unconverted
	calls = nil
	huge := put(t, d, "huge.tiff", append([]byte("II*\x00"), make([]byte, MaxConvertInputBytes)...))
	if _, err := s.Normalize(huge, stubConv(&calls)); err == nil || len(calls) != 0 || !strings.Contains(err.Error(), "the most that can be converted is 200 MB") {
		t.Errorf("huge tiff: %v %v", err, calls)
	}
	// a native-format image over 20 MB is still refused outright
	pngBig := put(t, d, "big.png", append(pngBytes(t, 2, 2), make([]byte, 21<<20)...))
	if _, err := s.Normalize(pngBig, stubConv(&calls)); err == nil || !strings.Contains(err.Error(), "the limit is 20 MB") {
		t.Errorf("big png: %v", err)
	}
}

func TestSniffGenericHEIFBrand(t *testing.T) {
	p := put(t, t.TempDir(), "x.dat", append([]byte{0, 0, 0, 24}, []byte("ftypmif1")...))
	if sniff(p) != "heif" {
		t.Errorf("sniff = %q", sniff(p))
	}
}

func TestNewPasteBaseIsReapable(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "att")}
	b, err := s.NewPasteBase()
	if err != nil || filepath.Dir(b) != s.Dir {
		t.Fatalf("%q %v", b, err)
	}
	for _, ext := range []string{".png", ".tiff"} {
		if !reapable.MatchString(filepath.Base(b + ext)) {
			t.Errorf("%s would never be reaped", filepath.Base(b+ext))
		}
	}
}
