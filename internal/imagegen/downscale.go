package imagegen

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	xdraw "golang.org/x/image/draw"
)

// Downscaling the input image before it is sent.
//
// This is a BILLING control, not a quality one. Several fal endpoints bill per
// MEGAPIXEL and round up, so the size of the input decides the price of the
// output: at $0.01/MP a 12 MP phone portrait costs $0.12 an image, not $0.01,
// and the 40 Mpx decode ceiling permits $0.40.
//
// The spike did this in Python and the port to Go dropped it, while the price
// table kept a comment asserting it happened — so the app quoted $0.06 for a
// six-image run that could bill $2.40. A quote that confident and that wrong is
// worse than no quote.
//
// One megapixel is what the spike validated the prompt behaviour against, so it
// is also the size the identity and framing findings actually describe.
const targetPixels = 1 << 20 // 1 Mpx

// downscaleToBudget returns raw scaled to at most targetPixels, or raw
// unchanged when it is already under and therefore cannot be costing extra.
//
// Re-encoded as JPEG because the result is an upload, never the pack's art: the
// generated image comes back from the provider and is stored losslessly. The
// input only has to carry the subject's identity.
func downscaleToBudget(raw []byte) ([]byte, string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode header: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxDim || cfg.Height > maxDim {
		return nil, "", fmt.Errorf("implausible image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	px := int64(cfg.Width) * int64(cfg.Height)
	if px > maxPixels {
		return nil, "", fmt.Errorf("image too large to decode: %d pixels (max %d)", px, maxPixels)
	}
	if px <= targetPixels {
		return raw, "", nil // already within budget; sending it untouched is free
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode: %w", err)
	}
	b := src.Bounds()
	// Scale by AREA, not by the longest side: the bill is megapixels, so the
	// area is the thing that has to come under budget whatever the aspect.
	ratio := math.Sqrt(float64(targetPixels) / float64(px))
	w := int(float64(b.Dx()) * ratio)
	h := int(float64(b.Dy()) * ratio)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// CatmullRom rather than the stdlib's nearest-neighbour: this image is the
	// sole carrier of the subject's identity, and aliasing the face is the one
	// artifact the whole prompt template is trying to prevent.
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 92}); err != nil {
		return nil, "", fmt.Errorf("encode: %w", err)
	}
	return out.Bytes(), "image/jpeg", nil
}
