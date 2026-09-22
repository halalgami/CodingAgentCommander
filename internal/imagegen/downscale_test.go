package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bigJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// A gradient, not a flat fill: a solid image compresses to almost nothing
	// and would make a size-based assertion meaningless.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: uint8((x + y) % 256), A: 255})
		}
	}
	draw.Draw(img, image.Rect(0, 0, w/4, h/4), &image.Uniform{C: color.RGBA{R: 255, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func dims(t *testing.T, raw []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

// The whole point: several fal endpoints bill per MEGAPIXEL and round up, so
// the input's size sets the output's price. Without this, a 12 Mpx phone
// portrait costs 12x what the app quotes.
func TestDownscaleBringsAnOversizedInputUnderBudget(t *testing.T) {
	raw := bigJPEG(t, 4000, 3000) // 12 Mpx
	out, mime, err := downscaleToBudget(raw)
	if err != nil {
		t.Fatal(err)
	}
	w, h := dims(t, out)
	if px := int64(w) * int64(h); px > targetPixels {
		t.Errorf("still %d pixels after downscale, budget is %d", px, targetPixels)
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q; the caller must be told the bytes were re-encoded", mime)
	}
	// Aspect must survive, or the subject is distorted before the model sees it.
	want, got := 4000.0/3000.0, float64(w)/float64(h)
	if got < want*0.98 || got > want*1.02 {
		t.Errorf("aspect %.3f, want ~%.3f", got, want)
	}
}

// An image already under budget costs nothing extra, so touching it would only
// add a lossy re-encode for no benefit.
func TestDownscaleLeavesASmallInputUntouched(t *testing.T) {
	raw := bigJPEG(t, 800, 600) // 0.48 Mpx
	out, mime, err := downscaleToBudget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("a within-budget image was re-encoded")
	}
	if mime != "" {
		t.Errorf("mime = %q; an untouched image keeps the caller's own type", mime)
	}
}

// A very wide or very tall image must come under budget by AREA. Scaling the
// longest side to a fixed length leaves a panorama far over the pixel budget.
func TestDownscaleBudgetsByAreaNotByLongestSide(t *testing.T) {
	for _, d := range [][2]int{{6000, 400}, {400, 6000}, {3000, 3000}} {
		out, _, err := downscaleToBudget(bigJPEG(t, d[0], d[1]))
		if err != nil {
			t.Fatalf("%dx%d: %v", d[0], d[1], err)
		}
		w, h := dims(t, out)
		if px := int64(w) * int64(h); px > targetPixels {
			t.Errorf("%dx%d -> %dx%d = %d px, over budget", d[0], d[1], w, h, px)
		}
	}
}

func TestDownscaleRejectsWhatTheDecoderShould(t *testing.T) {
	if _, _, err := downscaleToBudget([]byte("not an image")); err == nil {
		t.Error("accepted non-image bytes")
	}
	if _, _, err := downscaleToBudget(nil); err == nil {
		t.Error("accepted empty input")
	}
}

// The end-to-end guarantee: whatever the user picks, what crosses the wire is
// within budget. This is the assertion that actually protects the bill.
func TestGenerateNeverSendsMoreThanOneMegapixel(t *testing.T) {
	var gotPixels int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		arr, _ := body["image_urls"].([]any)
		if len(arr) != 1 {
			t.Errorf("image_urls had %d entries", len(arr))
			return
		}
		uri, _ := arr[0].(string)
		b64 := uri[strings.Index(uri, ",")+1:]
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		gotPixels = int64(cfg.Width) * int64(cfg.Height)

		art := solidJPEG(t, color.RGBA{R: 120, G: 120, B: 120, A: 255})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"data:image/jpeg;base64,` +
			base64.StdEncoding.EncodeToString(art) + `"}]}`))
	}))
	defer srv.Close()
	withFalBase(t, srv.URL)

	f := &Fal{}
	if _, err := f.Generate(context.Background(), "k", Request{
		Mode: ModeEdit, Base: bigJPEG(t, 4032, 3024), Prompt: "p", // a real phone photo
	}); err != nil {
		t.Fatal(err)
	}
	if gotPixels == 0 {
		t.Fatal("the server never saw an image")
	}
	if gotPixels > targetPixels {
		t.Errorf("sent %d pixels; at $0.01/MP that is ~%.0fx the quoted price",
			gotPixels, float64(gotPixels)/float64(targetPixels))
	}
}
